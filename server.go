package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

//go:embed static/*
var content embed.FS

const PROD = false

var (
	button = Button{
		Input:   rpio.Pin(14),
		Output:  rpio.Pin(15),
		Timeout: 20 * time.Second,
	}

	startedAt time.Time
)

func init() {
	err := rpio.Open()
	if err != nil {
		panic(err)
	}

	button.Input.Input()
	button.Input.PullUp()

	button.Output.Output()
	button.Output.Low()
}

func timestamp() time.Time {
	return time.Now().UTC().Round(time.Millisecond)
}

func jsonError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})
	log.Printf("caught error: %s", err.Error())
}

type ButtonPress struct {
	PressedAt  time.Time `json:"pressed_at"`
	Elapsed    float64   `json:"elapsed"`
	StartState bool      `json:"start_state"`
	EndState   bool      `json:"end_state"`
}

type Button struct {
	Input           rpio.Pin
	Output          rpio.Pin
	Timeout         time.Duration
	LastButtonPress ButtonPress

	pendingPress ButtonPress
	pressing     bool
	doneChan     chan ButtonPress
	cancel       context.CancelFunc
	mu           sync.RWMutex
}

func (b *Button) IsPressed() bool {
	return b.pressing
}

func (b *Button) IsOn() bool {
	return b.Input.Read() == rpio.High
}

func (b *Button) Press(ctx context.Context) (<-chan ButtonPress, error) {
	if b.IsPressed() {
		return nil, errors.New("button already pressed")
	}

	ctx, b.cancel = context.WithTimeout(ctx, b.Timeout)
	b.pressing = true
	if PROD {
		b.Output.High()
	}
	log.Println("button press")
	b.doneChan = make(chan ButtonPress, 1)
	b.pendingPress = ButtonPress{
		PressedAt:  timestamp(),
		StartState: b.IsOn(),
	}

	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			log.Println("encountered timeout")
		}
		b.Release()
	}()

	return b.doneChan, nil
}

func (b *Button) Release() (ButtonPress, error) {
	if !b.IsPressed() {
		return ButtonPress{}, errors.New("button already released")
	}

	b.pressing = false
	if PROD {
		b.Output.Low()
	}
	elapsed := time.Since(b.pendingPress.PressedAt).Round(time.Millisecond)
	log.Printf("button release after %s\n", elapsed)

	b.pendingPress.Elapsed = elapsed.Seconds()
	b.pendingPress.EndState = b.IsOn()
	b.LastButtonPress = b.pendingPress

	b.doneChan <- b.LastButtonPress
	close(b.doneChan)
	b.cancel()

	return b.LastButtonPress, nil
}

func handlePress(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	query := r.URL.Query()
	t := query.Get("t")
	wait := query.Has("wait")

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

	done, err := button.Press(ctx)
	if err != nil {
		jsonError(w, http.StatusTeapot, err)
		return
	}

	if wait {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(<-done)
	} else {
		w.WriteHeader(http.StatusAccepted)
	}
}

func handleRelease(w http.ResponseWriter, r *http.Request) {
	bp, err := button.Release()
	if err != nil {
		jsonError(w, http.StatusTeapot, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bp)
}

type statusResp struct {
	On           bool         `json:"on"`
	RunningSince time.Time    `json:"running_since"`
	LastPress    *ButtonPress `json:"last_press"`
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	bp := &button.LastButtonPress
	if bp.Elapsed == 0 {
		bp = nil
	}

	resp := statusResp{
		On:           button.IsOn(),
		RunningSince: startedAt,
		LastPress:    bp,
	}
	json.NewEncoder(w).Encode(resp)
	log.Println("sent state")
}

func main() {
	global := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Press-Timeout", fmt.Sprintf("%f", button.Timeout.Seconds()))
		http.DefaultServeMux.ServeHTTP(w, r)
	}

	sub, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}

	http.Handle("GET /", http.FileServerFS(sub))
	http.HandleFunc("GET /status", handleStatus)
	http.HandleFunc("POST /press", handlePress)
	http.HandleFunc("POST /release", handleRelease)

	log.Println("server online")
	startedAt = timestamp()
	log.Fatal(http.ListenAndServe(":5000", http.HandlerFunc(global)))
}
