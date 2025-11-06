package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/lithammer/fuzzysearch/fuzzy"
	_ "modernc.org/sqlite"
)

const (
	DBFile     = "Data/dom6api.db"
	APIPort    = 8002
	FuzzyScore = 70
)

var (
	tables          = []string{"items", "spells", "units", "sites", "mercs", "events"}
	cleanRe         = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)
	tableColumns    = map[string][]string{}
	tableColumnSets = make(map[string]map[string]struct{})
)

func initDB(filename string, tables []string) (*sql.DB, error) {
	memDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	if _, err := memDB.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS disk;", filename)); err != nil {
		return nil, fmt.Errorf("attach disk DB: %w", err)
	}

	for _, table := range tables {
		if _, err := memDB.Exec(fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM disk.%s;", table, table)); err != nil {
			return nil, fmt.Errorf("copy table %s: %w", table, err)
		}
		rows, err := memDB.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			return nil, err
		}
		cols := []string{}
		columnSet := make(map[string]struct{})
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, dflt_value, pk any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt_value, &pk); err != nil {
				continue
			}
			cols = append(cols, name)
			columnSet[name] = struct{}{}
		}
		rows.Close()
		tableColumns[table] = cols
		tableColumnSets[table] = columnSet
	}
	return memDB, nil
}

func serveScreenshot(table string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := strings.TrimPrefix(c.Path(), "/"+table+"/")
		id = strings.TrimSuffix(id, "/screenshot")
		id = cleanRe.ReplaceAllString(id, "")
		if id == "" {
			return fiber.NewError(fiber.StatusBadRequest, "missing id")
		}

		path := filepath.Join("Data", table, id+".png")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fiber.NewError(fiber.StatusNotFound, "file not found")
		}

		c.Type("png")
		return c.SendFile(path)
	}
}

func handleQuery(db *sql.DB, table string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		table = cleanRe.ReplaceAllString(table, "")
		if _, ok := tableColumnSets[table]; !ok {
			return fiber.NewError(fiber.StatusBadRequest, "unknown table")
		}
		if c.Method() != "GET" {
			return fiber.NewError(fiber.StatusMethodNotAllowed, "only GET allowed")
		}

		queryParams := c.Queries()
		enableFuzzy := queryParams["match"] == "fuzzy"
		delete(queryParams, "match")

		stmt, err := db.Prepare(fmt.Sprintf("SELECT * FROM %s", table))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		defer stmt.Close()
		rows, err := stmt.Query()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		defer rows.Close()

		cols, _ := rows.Columns()
		results := []map[string]any{}

	rowsLoop:
		for rows.Next() {
			values := make([]any, len(cols))
			for i := range values {
				values[i] = new(any)
			}
			if err := rows.Scan(values...); err != nil {
				continue
			}

			row := make(map[string]any, len(cols))
			for i, name := range cols {
				val := *(values[i].(*any))
				if b, ok := val.([]byte); ok {
					row[name] = string(b)
				} else {
					row[name] = val
				}
			}

			for key, vals := range queryParams {
				if _, ok := tableColumnSets[table][key]; !ok {
					return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("unknown column '%s'", key))
				}
				colVal := strings.ToLower(fmt.Sprint(row[key]))
				queryVal := strings.ToLower(cleanRe.ReplaceAllString(vals, ""))
				if (!enableFuzzy && colVal != queryVal) ||
					(enableFuzzy && !strings.Contains(colVal, queryVal) && fuzzy.RankMatch(queryVal, colVal) < FuzzyScore) {
					continue rowsLoop
				}
			}
			row["image"] = fmt.Sprintf("/%s/%v/screenshot", table, row["id"])
			results = append(results, row)
		}
		return c.JSON(fiber.Map{table: results})
	}
}

func StartServer(dbFile, addr string) error {

	db, err := initDB(dbFile, tables)
	if err != nil {
		return fmt.Errorf("DB init: %w", err)
	}

	app := fiber.New(fiber.Config{
		AppName: "PublicReadOnlyAPI",
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,OPTIONS",
	}))

	app.Use(limiter.New(limiter.Config{
		Max:        1000,
		Expiration: 1 * time.Minute,
	}))

	for _, table := range tables {
		t := table

		// Screenshot route
		app.Get("/"+t+"/:id/screenshot", func(c *fiber.Ctx) error {
			return serveScreenshot(t)(c)
		})

		// Query route supporting both /table/:id and /table?column=value
		app.Get("/"+t, handleQuery(db, t))
		app.Get("/"+t+"/:id", func(c *fiber.Ctx) error {
			id := cleanRe.ReplaceAllString(c.Params("id"), "")
			if id != "" {
				// inject into query params as "id"
				q := c.Queries()
				q["id"] = id
				c.Request().URI().SetQueryString("id=" + id)

			}
			return handleQuery(db, t)(c)
		})
	}

	log.Printf("Public read-only API listening on %s", addr)
	return app.Listen(addr)
}

func main() {
	fmt.Println(`
______ ________  ___   ____    ___  ______ _____ 
|  _  \  _  |  \/  |  / ___|  / _ \ | ___ \_   _|
| | | | | | | .  . | / /___  / /_\ \| |_/ / | |  
| | | | | | | |\/| | | ___ \ |  _  ||  __/  | |  
| |/ /\ \_/ / |  | | | \_/ | | | | || |    _| |_ 
|___/  \___/\_|  |_/ \_____/ \_| |_/\_|    \___/ 
                                                 
MIT License
Use, modify, and distribute freely, with credit to Monkeydew — the G.O.A.T.
                                                 `)

	if len(os.Args) > 1 && os.Args[1] == "build" {
		folder := "Data"

		// Remove existing folder
		os.RemoveAll(folder)

		// Optional: remove .git to save space
		os.RemoveAll(filepath.Join(folder, ".git"))

		log.Printf("Assets pulled into '%s'", folder)
	}

	log.Printf("Starting Go server on http://localhost:%d ...", APIPort)
	if err := StartServer(DBFile, fmt.Sprintf("localhost:%d", APIPort)); err != nil {
		log.Fatal("Go server failed:", err)
	}

}
