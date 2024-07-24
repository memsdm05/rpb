package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"flag"
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

var (
	argUser    *string        = flag.String("user", "", "Login user")
	argSecret  *string        = flag.String("secret", "", "Login password (required)")
	argAddr    *string        = flag.String("addr", ":5000", "Address to bind to")
	argTimeout *time.Duration = flag.Duration("timeout", 20*time.Second, "Maximum time a server waits before releasing a button")
	argInput   *int           = flag.Int("input", 14, "Pin used for input")
	argOutput  *int           = flag.Int("output", 15, "Pin used for output")
	argProd    *bool          = flag.Bool("prod", false, "Tells server to actually activate pins")
)

var (
	button    Button
	startedAt time.Time
)

func init() {
	err := rpio.Open()
	if err != nil {
		panic(err)
	}
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

func global(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	username, password, ok := r.BasicAuth()
	if ok {
		passwordHash := sha256.Sum256([]byte(password))
		expectedPasswordHash := sha256.Sum256([]byte(*argSecret))
		match := subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1

		if *argUser != "" {
			usernameHash := sha256.Sum256([]byte(*argUser))
			expectedUsernameHash := sha256.Sum256([]byte(username))
			match = match && subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1
		}

		if match {
			w.Header().Set("Press-Timeout", fmt.Sprintf("%f.2f", button.Timeout.Seconds()))
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

func main() {
	flag.Parse()

	if *argSecret == "" {
		log.Fatalln("secret must be supplied")
	}

	button = Button{
		Input:      rpio.Pin(*argInput),
		Output:     rpio.Pin(*argOutput),
		Production: *argProd,
		Timeout:    *argTimeout,
	}

	button.Input.Input()
	button.Input.PullUp()

	button.Output.Output()
	button.Output.Low()

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
	err = rpio.Close()
	if err != nil {
		panic(err)
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	} else {
		log.Println("Shutdown complete")
	}
}
