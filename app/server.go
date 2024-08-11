package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

var (
	button    Button
	startedAt time.Time
	db        *sql.DB
	srv       *http.Server
)

func init() {
	err := rpio.Open()
	if err != nil {
		Config.Production = false
		log.Println("Cannot connect to gpio memory, turning off production mode")
	}
}

func timestamp() time.Time {
	return time.Now().UTC().Round(time.Millisecond)
}

func jsonResp(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, err error) {
	jsonResp(w, code, map[string]string{
		"error": err.Error(),
	})
	log.Printf("Caught error: %s", err.Error())
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Recovery caught error: %s", err)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler, secret string) http.Handler {
	expectedPasswordHash := sha256.Sum256([]byte(secret))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok {
			passwordHash := sha256.Sum256([]byte(password))
			match := subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1

			if match {
				w.Header().Set("Press-Timeout", fmt.Sprintf("%f.2f", button.Timeout.Seconds()))
				next.ServeHTTP(w, r)
				return
			} else {
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

				db.Exec(
					`INSERT INTO bad_access (timestamp, ip, country, username, password) VALUES (?, ?, ?, ?, ?)`,
					timestamp(), ip, country, username, password,
				)

				log.Printf("Attempted access from %s (%s)\n", ip, country)
			}
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(w, r)
	})
}

func setupRoutes(fsys fs.FS) (handler http.Handler) {
	mux := http.NewServeMux()

	mux.Handle("GET /", http.FileServerFS(fsys))

	mux.HandleFunc("GET /status", handleStatus)

	mux.HandleFunc("POST /press", handlePress)
	mux.HandleFunc("GET /press/history", handlePressHistory)
	mux.HandleFunc("POST /release", handleRelease)

	mux.HandleFunc("POST /turn/{state}", handleTurn)
	mux.HandleFunc("GET /state", handleState)
	mux.HandleFunc("GET /state/history", handleStateHistory)

	handler = authMiddleware(mux, Config.Secret)
	handler = recoveryMiddleware(handler)
	handler = corsMiddleware(handler)

	return
}

func setupButton() {
	var backend Backend
	if Config.Production {
		backend = &RpioBackend{
			Input:  rpio.Pin(Config.PinInput),
			Output: rpio.Pin(Config.PinOutput),
		}
	} else {
		backend = &DummyBackend{}
	}

	button = Button{
		Backend: backend,
		Timeout: Config.Timeout,
	}
	button.Setup()
}

func Start(content embed.FS) {
	db = CreateDB(Config.DBPath)
	setupButton()

	sub, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}

	srv = &http.Server{
		Handler: setupRoutes(sub),
		Addr:    Config.Addr,
	}

	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections")
	}()

	log.Println("Server online")
	startedAt = timestamp()

	db.Exec(
		`INSERT INTO startup (started_at, timeout, input_pin, output_pin, prod) VALUES (?, ?, ?, ?, ?)`,
		startedAt,
		button.Timeout.Seconds(),
		Config.PinInput,
		Config.PinOutput,
		Config.Production,
	)
}

func Stop() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	defer db.Close()
	if Config.Production {
		defer rpio.Close()
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	} else {
		log.Println("Shutdown complete")
	}
}
