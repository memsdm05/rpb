package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
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

var config = struct {
	DBPath     string
	Secret     string
	Addr       string
	Timeout    time.Duration
	PinInput   int
	PinOutput  int
	Production bool
}{}

var (
	button    Button
	startedAt time.Time
	db        *sql.DB
)

func init() {
	err := rpio.Open()
	if err != nil {
		config.Production = false
		log.Println("Cannot connect to gpio memory, turning off production mode")
	}

	flag.StringVar(&config.Secret, "secret", "", "Login password (required)")
	flag.StringVar(&config.DBPath, "db", "./rpb.db", "Where the database is")
	flag.StringVar(&config.Addr, "addr", ":5000", "Address to bind to")
	flag.IntVar(&config.PinInput, "input", 14, "Pin used for input")
	flag.IntVar(&config.PinOutput, "output", 15, "Pin used for output")
	flag.BoolVar(&config.Production, "prod", false, "Tells server to actually activate pins")
	flag.DurationVar(&config.Timeout, "timeout", 20*time.Second, "Maximum time a server waits before releasing a button")
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

func global(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	username, password, ok := r.BasicAuth()
	if ok {
		passwordHash := sha256.Sum256([]byte(password))
		expectedPasswordHash := sha256.Sum256([]byte(config.Secret))
		match := subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1

		if match {
			w.Header().Set("Press-Timeout", fmt.Sprintf("%f.2f", button.Timeout.Seconds()))
			http.DefaultServeMux.ServeHTTP(w, r)
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
}

func stateWatcher() {
	state := button.IsOn()
	for {
		current := button.IsOn()
		if current != state {
			db.Exec(
				`INSERT INTO state (changed_at, is_on, during_press) VALUES (?, ?, ?)`,
				timestamp(), current, button.IsPressed(),
			)
			if current {
				log.Println("State is now on")
			} else {
				log.Println("State is now off")
			}

			state = current
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func main() {
	flag.Parse()
	db = CreateDb(config.DBPath)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	if config.Secret == "" {
		config.Secret = os.Getenv("RPB_SECRET")
	}
	if config.Secret == "" || config.Secret == "<INSERT SECRET HERE>" {
		log.Fatalln("secret must be supplied")
	}

	button = Button{
		Input:      rpio.Pin(config.PinInput),
		Output:     rpio.Pin(config.PinOutput),
		Production: config.Production,
		Timeout:    config.Timeout,
	}
	button.Setup()
	go stateWatcher()

	sub, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}

	http.Handle("GET /", http.FileServerFS(sub))
	http.HandleFunc("GET /status", handleStatus)
	http.HandleFunc("POST /press", handlePress)
	http.HandleFunc("POST /release", handleRelease)
	http.HandleFunc("GET /history", handleHistory)

	srv := &http.Server{
		Handler: http.HandlerFunc(global),
		Addr:    config.Addr,
	}

	log.Println("Server online")
	startedAt = timestamp()

	_, err = db.Exec(
		`INSERT INTO startup (started_at, timeout, input_pin, output_pin, prod) VALUES (?, ?, ?, ?, ?)`,
		startedAt,
		button.Timeout.Seconds(),
		config.PinInput,
		config.PinOutput,
		button.Production,
	)
	if err != nil {
		panic(err)
	}

	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections")
	}()

	<-sigs

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	defer db.Close()
	if button.Production {
		defer rpio.Close()
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	} else {
		log.Println("Shutdown complete")
	}
}
