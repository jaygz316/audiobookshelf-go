package main

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

func BenchmarkRecomputeIgnorePrefixes(b *testing.B) {
	db, err := sql.Open("sqlite", "file:benchmark.db?mode=memory&cache=shared")
	if err != nil {
		b.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE books (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT);
		CREATE TABLE podcasts (id TEXT PRIMARY KEY, title TEXT, titleIgnorePrefix TEXT);
		CREATE TABLE series (id TEXT PRIMARY KEY, name TEXT, nameIgnorePrefix TEXT);
	`)
	if err != nil {
		b.Fatalf("Failed to create tables: %v", err)
	}

	numItems := 1000
	tx, err := db.Begin()
	if err != nil {
		b.Fatalf("Failed to begin tx: %v", err)
	}
	for i := 0; i < numItems; i++ {
		_, _ = tx.Exec("INSERT INTO books (id, title, titleIgnorePrefix) VALUES (?, ?, ?)", fmt.Sprintf("b%d", i), fmt.Sprintf("The Book %d", i), "")
		_, _ = tx.Exec("INSERT INTO podcasts (id, title, titleIgnorePrefix) VALUES (?, ?, ?)", fmt.Sprintf("p%d", i), fmt.Sprintf("A Podcast %d", i), "")
		_, _ = tx.Exec("INSERT INTO series (id, name, nameIgnorePrefix) VALUES (?, ?, ?)", fmt.Sprintf("s%d", i), fmt.Sprintf("An Series %d", i), "")
	}
	tx.Commit()

	prefixes := []string{"the", "a", "an"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.Exec("UPDATE books SET titleIgnorePrefix = title;")
		db.Exec("UPDATE podcasts SET titleIgnorePrefix = title;")
		db.Exec("UPDATE series SET nameIgnorePrefix = name;")

		b.StartTimer()
		recomputeIgnorePrefixes(db, prefixes)
		b.StopTimer()
	}
}
