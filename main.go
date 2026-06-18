package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
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
	idRe            = regexp.MustCompile(`^[a-zA-Z0-9 ]{1,64}$`)
	tableColumns    = map[string][]string{}
	tableColumnSets = make(map[string]map[string]struct{})
	logfile         = "dom6api.log"
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
			var notnull, dfltValue, pk any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
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
		id := c.Params("id")
		if !idRe.MatchString(id) {
			return fiber.NewError(fiber.StatusBadRequest, "invalid id")
		}
		base := filepath.Join("Data", table)
		path := filepath.Join(base, id+".png")
		absBase, _ := filepath.Abs(base)
		absPath, _ := filepath.Abs(path)
		if !strings.HasPrefix(absPath, absBase+string(os.PathSeparator)) {
			return fiber.NewError(fiber.StatusForbidden, "access denied")
		}
		fi, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fiber.NewError(fiber.StatusNotFound, "file not found")
			}
			return fiber.NewError(fiber.StatusInternalServerError, "file error")
		}
		if fi.Size() > 10*1024*1024 {
			return fiber.NewError(fiber.StatusForbidden, "file too large")
		}
		c.Type("png")
		f, _ := os.Open(absPath)
		defer f.Close()
		_, err = io.Copy(c.Response().BodyWriter(), f)
		return err
	}
}

func handleQuery(db *sql.DB, table string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if _, ok := tableColumnSets[table]; !ok {
			return fiber.NewError(fiber.StatusNotFound, "unknown table")
		}
		if c.Method() != "GET" {
			return fiber.NewError(fiber.StatusMethodNotAllowed, "only GET allowed")
		}

		queryParams := c.Queries()
		enableFuzzy := queryParams["match"] == "fuzzy"
		delete(queryParams, "match")

		if len(queryParams) > 5 {
			return fiber.NewError(fiber.StatusBadRequest, "too many params")
		}
		for k, v := range queryParams {
			if len(v) > 256 {
				return fiber.NewError(fiber.StatusBadRequest, "param too long")
			}
			if _, ok := tableColumnSets[table][k]; !ok {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("unknown column '%s'", k))
			}
		}

		ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
		defer cancel()
		stmt, err := db.PrepareContext(ctx, fmt.Sprintf("SELECT * FROM %s", table))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		defer stmt.Close()

		rows, err := stmt.QueryContext(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		defer rows.Close()

		cols, _ := rows.Columns()
		results := []map[string]any{}
		const maxResults = 200
		const maxScanned = 10000
		scanned := 0

	rowsLoop:
		for rows.Next() {
			scanned++
			if scanned > maxScanned {
				break
			}

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
				colVal := strings.ToLower(fmt.Sprint(row[key]))
				queryVal := strings.ToLower(vals)
				if (!enableFuzzy && colVal != queryVal) ||
					(enableFuzzy && !strings.Contains(colVal, queryVal) && fuzzy.RankMatch(queryVal, colVal) < FuzzyScore) {
					continue rowsLoop
				}
			}

			row["image"] = fmt.Sprintf("/%s/%v/screenshot", table, row["id"])
			results = append(results, row)
			if len(results) >= maxResults {
				break
			}
		}

		if len(results) == 0 {
			return fiber.NewError(fiber.StatusNotFound, "no matches found")
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

	file, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("log file open: %w", err)
	}
	defer file.Close()

	app.Use(logger.New(logger.Config{
		Output: file,
	}))

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
		app.Get("/"+t+"/:id/screenshot", serveScreenshot(t))
		app.Get("/"+t, handleQuery(db, t))
		app.Get("/"+t+"/:id", func(c *fiber.Ctx) error {
			id := c.Params("id")
			if id != "" {
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
