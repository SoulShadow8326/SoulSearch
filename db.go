package main

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB() {
	err := os.MkdirAll("./data", os.ModePerm)
	if err != nil {
		log.Fatal("Failed to create data folder:", err)
	}

	db, err := sql.Open("sqlite3", "./data/data.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	createStmt := `
	CREATE TABLE IF NOT EXISTS pages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT UNIQUE,
		title TEXT,
		content TEXT,
		crawled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS links (
		from_url TEXT,
		to_url TEXT
	);	
	`

	_, err = db.Exec(createStmt)
	if err != nil {
		log.Fatal("Failed to create table:", err)
	}

	DB = db
}

func StorePageData(page PageData) error {
	stmt := `
	INSERT OR IGNORE INTO pages (url, title, content)
	VALUES (?, ?, ?);
	`
	_, err := DB.Exec(stmt, page.URL, page.Title, page.Content)
	return err
}

func StoreLinks(from string, to []string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO links (from_url, to_url) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, link := range to {
		_, err := stmt.Exec(from, link)
		if err != nil {
			log.Println("Failed to insert link:", err)
		}
	}
	return tx.Commit()
}