package main

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS press (
	id INTEGER PRIMARY KEY,
	source TEXT,
	pressed_at TEXT,
	elapsed REAL,
	start_state INTEGER,
	end_state INTEGER
);

CREATE TABLE IF NOT EXISTS state (
	changed_at TEXT,
	is_on INTEGER,
	during_press INTEGER
);

CREATE TABLE IF NOT EXISTS bad_access (
	timestamp TEXT,
	ip TEXT,
	country TEXT,
	username TEXT,
	password TEXT
);

CREATE TABLE IF NOT EXISTS startup (
	started_at TEXT,
	timeout REAL,
	input_pin INTEGER,
	output_pin INTEGER,
	prod INTEGER
);
`

var (
	InsertPress *sql.Stmt
)

func CreateDb(path string) *sql.DB {
	var err error
	db, err = sql.Open("sqlite3", path)
	if err != nil {
		panic(err)
	}

	if _, err = db.Exec(schema); err != nil {
		panic(err)
	}

	return db
}
