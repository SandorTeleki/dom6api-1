package main

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	_ "embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	DBFile        = "Data/dom6api.db"
	InspectorPort = 8001
	APIPort       = 8002
)

//go:embed create_tables.sql
var sqlFile []byte

func dbcheck(filename string) *sql.DB {

	for _, cat := range []string{"events", "items", "mercs", "sites", "spells", "units"} {
		if err := os.MkdirAll(filepath.Join("Data", cat), os.ModePerm); err != nil {
			log.Fatalf("could not create folder %s: %v", cat, err)
		}
	}

	log.Println("Opening database:", filename)
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		log.Fatalf("dbcheck: failed to open database: %v", err)
	}

	if _, err := db.Exec(string(sqlFile)); err != nil {
		log.Fatalf("dbcheck: failed to execute SQL: %v", err)
	}

	log.Println("dbcheck: SQL executed successfully")
	return db
}
func tarFolder(src, dest string) error {
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer file.Close()

	gw := gzip.NewWriter(file)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, path)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(filepath.Dir(src), path)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			if err != nil {
				return err
			}
		}
		return nil
	})
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
	db := dbcheck(DBFile)
	defer db.Close()

	folder := "dom6inspector"
	os.RemoveAll(folder)
	if err := exec.Command("git", "clone", "https://github.com/larzm42/dom6inspector", folder).Run(); err != nil {
		log.Fatal("Clone failed:", err)
	}

	pyCmd := exec.Command("python", "-m", "http.server", fmt.Sprint(InspectorPort))
	pyCmd.Dir, pyCmd.Stdout, pyCmd.Stderr = folder, os.Stdout, os.Stderr
	if err := pyCmd.Start(); err != nil {
		log.Fatal("Python server failed:", err)
	}
	log.Printf("Python server at http://localhost:%d (PID %d)", InspectorPort, pyCmd.Process.Pid)
	go pyCmd.Wait()

	for i := 0; i < 10; i++ {
		if resp, err := http.Get(fmt.Sprintf("http://localhost:%d", InspectorPort)); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	Scrape()
	log.Println("Scrape complete.")

	// Zip the Data folder
	if err := tarFolder("Data", "Data.zip"); err != nil {
		log.Fatalf("Failed to zip Data folder: %v", err)
	}
	log.Println("Data folder zipped as Data.zip")
}
