package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
	_ "modernc.org/sqlite"
)

var (
	schoolMap = map[float64]string{
		0: "Conjuration",
		1: "Alteration",
		2: "Evocation",
		3: "Construction",
		4: "Enchantment",
		5: "Thaumaturgy",
		6: "Blood",
		7: "Divine",
	}

	rarities = map[int]string{
		0:  "Common",
		1:  "Uncommon",
		2:  "Rare",
		5:  "Never random",
		11: "Throne lvl1",
		12: "Throne lvl2",
		13: "Throne lvl3",
	}
	categoryFields = map[string][]string{
		"item":  {"id", "name", "type", "constlevel", "mainlevel", "mpath", "gemcost"},
		"spell": {"id", "name", "gemcost", "mpath", "type", "school", "researchlevel"},
		"unit":  {"id", "name", "hp", "size", "mountmnr", "coridermnr"},
		"merc":  {"id", "name", "bossname", "com", "unit", "nrunits"},
		"site":  {"id", "name", "path", "level", "rarity"},
		"event": {"id", "name"},
	}
)

func Scrape() {

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("main: could not start Playwright: %v", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args:     []string{"--disable-gpu", "--no-sandbox", "--disable-dev-shm-usage"},
	})

	if err != nil {
		log.Fatalf("main: could not launch browser: %v", err)
	}

	page, err := browser.NewPage()
	if err != nil {
		log.Fatalf("main: could not create page: %v", err)
	}
	page.SetViewportSize(800, 600)

	if _, err = page.Goto("http://localhost:8001/?loadEvents=1", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Fatalf("main: could not goto url: %v", err)
	}

	db, err := sql.Open("sqlite", "Data/dom6api.db")
	if err != nil {
		log.Fatalf("main: could not open db: %v", err)
	}

	for cat := range categoryFields {
		fmt.Printf("Processing category: %s\n", cat)

		page.Click(fmt.Sprintf("#%s-page-button", cat))
		page.WaitForFunction(fmt.Sprintf(`
        () => DMI.modctx['%sdata'] && DMI.modctx['%sdata'].length > 0
    `, cat, cat), playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(10000)})

		// sort once per category by id
		_, errSort := page.Evaluate(fmt.Sprintf(`() => {
		DMI.modctx['%sdata'].sort((a, b) => a.id - b.id);
	}`, cat))
		if errSort != nil {
			log.Printf("Failed to sort %sdata: %v", cat, errSort)
		}

		countAny, _ := page.Evaluate(fmt.Sprintf(`() => DMI.modctx['%sdata'].length`, cat))
		count := int(countAny.(int))
		selector := fmt.Sprintf("#%s-page div.fixed-overlay", cat)
		jsCategoryFieldsJSON, _ := json.Marshal(categoryFields)

		for i := 0; i < count; i++ {
			var entity map[string]interface{}
			var err error

			for retries := 0; retries < 2; retries++ {
				entityJSON, errEval := page.Evaluate(fmt.Sprintf(`(i)=>{
				const e = DMI.modctx['%sdata'][i];
				const overlay = document.querySelector('#%s-page div.fixed-overlay');
				return new Promise((resolve, reject)=>{
					const start = Date.now();
					const f = ()=>{
						try {
							const rendered = e.renderOverlay(e);
							if(rendered instanceof Node){ overlay.innerHTML = ""; overlay.appendChild(rendered); }
							else overlay.innerHTML = String(rendered || "");

							if(overlay.innerHTML.trim().length > 0){
								const fieldsByCategory = %s;
								const catFields = fieldsByCategory["%s"];
								const data = {};
								catFields.forEach(f => { data[f] = e[f]; });
								resolve(JSON.stringify(data));
							}
							else if(Date.now() - start > 5000) reject("timeout waiting for overlayHTML");
							else setTimeout(f, 200);
						} catch(err) { reject(err); }
					};
					f();
				});
			}`, cat, cat, jsCategoryFieldsJSON, cat), i)

				if errEval == nil {
					err = json.Unmarshal([]byte(entityJSON.(string)), &entity)
					if err == nil {
						break
					} else {
						fmt.Printf("Failed to parse entity %d JSON: %v\n", i, err)
					}
				} else {
					err = errEval
					fmt.Printf("Retrying render for entity %d: %v\n", i, err)
					time.Sleep(500 * time.Millisecond)
				}
			}

			if err != nil {
				fmt.Printf("Failed to render entity %d after retries: %v\n", i, err)
				continue
			}

			path := filepath.Join("Data", cat+"s", fmt.Sprintf("%v.png", entity["id"]))
			if el, err := page.QuerySelector(selector); err == nil && el != nil {
				el.Screenshot(playwright.ElementHandleScreenshotOptions{Path: playwright.String(path)})
			}

			populate(db, cat, entity)
			fmt.Printf("[%s] [%s] Rendered entity %d/%d\n", time.Now().Format("2006-01-02 15:04:05"), cat, i+1, count)
		}
	}

	log.Println("main: Done. Closing browser...")
}

func populate(db *sql.DB, category string, entityMap map[string]interface{}) {

	switch category {
	case "spell":
		if entityMap["gemcost"] == nil {
			entityMap["gemcost"] = "0"
		}
		if s := entityMap["school"].(string); s == "-1" {
			return
		} else if f, err := strconv.ParseFloat(s, 64); err == nil {
			if mapped := schoolMap[f]; mapped != "" {
				entityMap["school"] = mapped
			}
		}

	case "site":
		if r, ok := entityMap["rarity"]; ok && r != nil {
			switch v := r.(type) {
			case int:
				entityMap["rarity"] = rarities[v]
			case float64:
				entityMap["rarity"] = rarities[int(v)]
			case string:
				if n, err := strconv.Atoi(v); err == nil {
					entityMap["rarity"] = rarities[n]
				}
			}
		}
	}

	fields, ok := categoryFields[category]
	if !ok {
		log.Printf("populate: unknown category: %s", category)
		return
	}

	insertSQL := fmt.Sprintf(
		"INSERT OR REPLACE INTO %ss (%s) VALUES (%s)",
		category,
		strings.Join(fields, ", "),
		strings.Repeat("?, ", len(fields)-1)+"?",
	)

	stmt, err := db.Prepare(insertSQL)
	if err != nil {
		log.Printf("populate: could not prepare insert for %s: %v", category, err)
		return
	}

	values := make([]interface{}, len(fields))
	for i, f := range fields {
		switch v := entityMap[f].(type) {
		case float64:
			values[i] = int(v)
		default:
			values[i] = v
		}
	}

	if _, err := stmt.Exec(values...); err != nil {
		log.Printf("populate: exec error for %s: %v", category, err)
	}
}
