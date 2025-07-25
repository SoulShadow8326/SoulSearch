package main

import (
	"database/sql"
	"log"
	"os"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB
var DBMutex sync.Mutex

func InitDB() {
	err := os.MkdirAll("./data", os.ModePerm)
	if err != nil {
		log.Fatal("Failed to create data folder:", err)
	}

	db, err := sql.Open("sqlite3", "./data/data.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	createPages := `
	CREATE TABLE IF NOT EXISTS pages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT UNIQUE,
		title TEXT,
		content TEXT,
		crawled BOOLEAN DEFAULT 0,
		crawled_at TIMESTAMP,
		pagerank REAL DEFAULT 1.0
	);
	`

	createLinks := `
	CREATE TABLE IF NOT EXISTS links (
		from_id INTEGER,
		to_id INTEGER,
		UNIQUE(from_id, to_id),
		FOREIGN KEY(from_id) REFERENCES pages(id),
		FOREIGN KEY(to_id) REFERENCES pages(id)
	);
	`
	createIndex := `
	CREATE TABLE IF NOT EXISTS inverted_index (
		term TEXT,
		page_id INTEGER,
		frequency INTEGER,
		UNIQUE(term, page_id),
		FOREIGN KEY(page_id) REFERENCES pages(id)
	);
	`

	_, err = db.Exec(createPages)
	if err != nil {
		log.Fatal("Failed to create pages table:", err)
	}
	_, err = db.Exec(createLinks)
	if err != nil {
		log.Fatal("Failed to create links table:", err)
	}
	_, err = db.Exec(createIndex)
	if err != nil {
		log.Fatal("Failed to create inverted_index table:", err)
	}

	DB = db
}

func StorePageData(page PageData) error {
	DBMutex.Lock()
	defer DBMutex.Unlock()
	_, err := DB.Exec(`INSERT OR IGNORE INTO pages (url) VALUES (?)`, page.URL)
	if err != nil {
		return err
	}

	_, err = DB.Exec(`
		UPDATE pages SET title = ?, content = ?, crawled = 1, crawled_at = ? WHERE url = ?;
	`, page.Title, page.Content, time.Now(), page.URL)
	if err != nil {
		return err
	}

	var pageID int
	err = DB.QueryRow(`SELECT id FROM pages WHERE url = ?`, page.URL).Scan(&pageID)
	if err != nil {
		return err
	}

	tokens := tokenize(page.Content)
	counts := map[string]int{}
	for _, t := range tokens {
		counts[t]++
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO inverted_index (term, page_id, frequency) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for term, freq := range counts {
		_, err := stmt.Exec(term, pageID, freq)
		if err != nil {
			log.Println("Insert inverted index error:", err)
		}
	}

	return tx.Commit()
}

func StoreLinks(from string, to []string) error {
	DBMutex.Lock()
	defer DBMutex.Unlock()
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Commit()

	var fromID int
	err = tx.QueryRow(`SELECT id FROM pages WHERE url = ?`, from).Scan(&fromID)
	if err != nil {
		log.Println("from_id not found:", from)
		return nil
	}

	stmtInsertPage, err := tx.Prepare(`INSERT OR IGNORE INTO pages (url) VALUES (?)`)
	if err != nil {
		return err
	}
	defer stmtInsertPage.Close()

	stmtInsertLink, err := tx.Prepare(`INSERT OR IGNORE INTO links (from_id, to_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmtInsertLink.Close()

	for _, dest := range to {
		_, _ = stmtInsertPage.Exec(dest)

		var toID int
		err := tx.QueryRow(`SELECT id FROM pages WHERE url = ?`, dest).Scan(&toID)
		if err == nil {
			_, _ = stmtInsertLink.Exec(fromID, toID)
		}
	}

	return nil
}

func LoadVisitedFromDB() {
	rows, err := DB.Query("SELECT url FROM pages WHERE crawled = 1")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err == nil {
			visited[url] = true
		}
	}
}

func QueueLinks(links []string) {
	DBMutex.Lock()
	defer DBMutex.Unlock()
	for _, link := range links {
		_, err := DB.Exec(`INSERT OR IGNORE INTO pages (url) VALUES (?)`, link)
		if err != nil {
			log.Println("Failed to queue link:", err)
		}
	}
}
