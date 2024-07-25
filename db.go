package main

const schema = `
CREATE TABLE IF NOT EXISTS press (
	id INTEGER PRIMARY KEY,
	source TEXT,
	pressed_at TEXT,
	elapsed INTEGER,
	start_state INTEGER,
	end_state INTEGER,
	caused_state INTEGER,
	FOREIGN KEY(caused_state) REFERENCES state(id)
);

CREATE TABLE IF NOT EXISTS state (
	id INTEGER PRIMARY KEY,
	start INTEGER,
	end INTEGER,
	was_remote INTEGER,
);

CREATE TABLE IF NOT EXISTS access (

);

`
