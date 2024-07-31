package app

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func handlePress(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	query := r.URL.Query()

	t := query.Get("t")
	wait := query.Has("wait")

	source, _, _ := r.BasicAuth()
	if source == "" {
		source = "unknown"
	}

	if t != "" {
		wait = true
		seconds, err := strconv.ParseFloat(t, 64)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}

		dur := time.Duration(seconds*1000) * time.Millisecond
		if dur > button.Timeout {
			jsonError(w, http.StatusBadRequest, fmt.Errorf("t (%s) cannot be longer than %s", dur, button.Timeout))
			return
		}

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(r.Context(), dur)
		defer cancel()
	}

	done, err := button.Press(source, ctx)
	if err != nil {
		jsonError(w, http.StatusTeapot, err)
		return
	}

	if wait {
		jsonResp(w, http.StatusOK, <-done)
	} else {
		jsonResp(w, http.StatusAccepted, map[string]float64{
			"timeout": button.Timeout.Seconds(),
		})
	}
}

func handleRelease(w http.ResponseWriter, r *http.Request) {
	bp, err := button.Release()
	if err != nil {
		jsonError(w, http.StatusTeapot, err)
		return
	}

	jsonResp(w, http.StatusOK, bp)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	bp := &button.LastButtonPress
	if bp.Elapsed == 0 {
		bp = nil
	}

	resp := struct {
		On           bool         `json:"on"`
		Pressed      bool         `json:"pressed"`
		RunningSince time.Time    `json:"running_since"`
		LastPress    *ButtonPress `json:"last_press"`
	}{
		On:           button.IsOn(),
		Pressed:      button.IsPressed(),
		RunningSince: startedAt,
		LastPress:    bp,
	}

	jsonResp(w, http.StatusOK, resp)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, cursor, err := PaginationParams(r, 10)
	if err != nil {
		jsonError(w, 400, err)
	}

	paginator := Paginator[ButtonPress]{
		Table: "press",
		Resolver: func(row ActualScanner) (ButtonPress, int, error) {
			bp, err := ButtonPressFromRow(row)
			return bp, int(bp.Id), err
		},
	}

	page, err := paginator.Paginate(ctx, limit, cursor)
	if err != nil {
		jsonError(w, 400, err)
		return
	}
	jsonResp(w, 200, page)
}
