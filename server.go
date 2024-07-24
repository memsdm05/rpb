package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

//go:embed static/*
var content embed.FS

const (
	PASSWORD = "changeme"
	PROD     = false
)

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
	log.Printf("Caught error: %s", err.Error())
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
	Pressed      bool         `json:"pressed"`
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
		Pressed:      button.IsPressed(),
		RunningSince: startedAt,
		LastPress:    bp,
	}
	json.NewEncoder(w).Encode(resp)
}

func main() {
	global := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		_, password, ok := r.BasicAuth()
		if ok {
			// usernameHash := sha256.Sum256([]byte(username))
			passwordHash := sha256.Sum256([]byte(password))
			// expectedUsernameHash := sha256.Sum256([]byte(USERNAME))
			expectedPasswordHash := sha256.Sum256([]byte(PASSWORD))

			// usernameMatch := (subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1)
			passwordMatch := subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1

			if passwordMatch {
				w.Header().Set("Press-Timeout", fmt.Sprintf("%f", button.Timeout.Seconds()))
				http.DefaultServeMux.ServeHTTP(w, r)
				return
			}

		}

		country := r.Header.Get("Cf-Ipcountry")
		if country == "" {
			country = "unknown"
		}

		ip := r.Header.Get("Cf-Connecting-Ip")
		if ip == "" {
			ip = r.Header.Get("X-Forwarded-For")
		}
		if ip == "" {
			ip = "unknown ip"
		}

		log.Printf("Attempted access from %s (%s)\n", ip, country)
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}

	sub, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	srv := &http.Server{
		Handler: http.HandlerFunc(global),
		Addr:    ":5000",
	}

	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections")
	}()

	http.Handle("GET /", http.FileServerFS(sub))
	http.HandleFunc("GET /status", handleStatus)
	http.HandleFunc("POST /press", handlePress)
	http.HandleFunc("POST /release", handleRelease)

	log.Println("Server online")
	startedAt = timestamp()

	<-sigs
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	} else {
		log.Println("Shutdown complete")
	}
}
