package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

func PaginationParams(r *http.Request, maxLimit int) (limit, cursor int, err error) {
	limit = 100
	cursor = 0
	query := r.URL.Query()

	if query.Has("limit") {
		limit, err = strconv.Atoi(query.Get("limit"))
		if limit > maxLimit {
			limit = maxLimit
		}
	}

	if query.Has("cursor") {
		cursor, err = strconv.Atoi(query.Get("cursor"))
	}

	return
}

type Page[T any] struct {
	Items       []T  `json:"items"`
	Limit      int  `json:"limit"`
	Cursor     int  `json:"cursor"`
	NextCursor *int `json:"next_cursor"`
}

type Paginator[T any] struct {
	Table        string
	Resolver     func(ActualScanner) (T, int, error)
	IncludeRowId bool
}

func (p *Paginator[T]) Paginate(ctx context.Context, limit, cursor int) (Page[T], error) {
	var (
		lastId int
		data   T
		err    error
	)

	// yes i know...
	query := "SELECT "
	if p.IncludeRowId {
		query += "rowid, "
	}
	query += fmt.Sprintf("* FROM %s WHERE rowid >= ? ORDER BY rowid ASC LIMIT ?", p.Table)

	page := Page[T]{
		Limit:  limit,
		Cursor: cursor,
	}

	rows, err := db.QueryContext(ctx, query, cursor, limit+1)
	if err != nil {
		return page, nil
	}

	for rows.Next() {
		if rows.Err() != nil {
			return page, rows.Err()
		}

		data, lastId, err = p.Resolver(rows)
		if err != nil {
			return page, err
		}

		page.Items = append(page.Items, data)
	}
	page.Items = page.Items[:len(page.Items)-1]
	if len(page.Items) == limit {
		page.NextCursor = &lastId
	}

	return page, nil

}
