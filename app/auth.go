package app

import "time"

type Session struct {
	Token     string
	Source    string
	CreatedAt time.Time
}

type Authentication struct {
	sessions map[string]Session
}
