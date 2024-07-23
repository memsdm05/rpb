package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

var (
	button = Button{
		Input:   rpio.Pin(14),
		Output:  rpio.Pin(15),
		Timeout: 20 * time.Second,
	}

	startedAt time.Time
	PROD      = false
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

func durToFloat(d time.Duration) float64 {
	return float64(d.Milliseconds()) / 1000
}

func floatToDur(d float64) time.Duration {
	return time.Duration(d*1000) * time.Millisecond
}

type ButtonPress struct {
	PressedAt  time.Time `json:"pressed_at"`
	Elapsed    int       `json:"elapsed"`
	StartState bool      `json:"start_state"`
	EndState   bool      `json:"end_state"`
}

type Button struct {
	Input           rpio.Pin
	Output          rpio.Pin
	Timeout         time.Duration
	LastButtonPress ButtonPress

	pendingPress ButtonPress

	doneChan chan ButtonPress
	cancel   context.CancelFunc
	mu       sync.RWMutex
}

func (b *Button) IsPressed() bool {
	return b.doneChan != nil
}

func (b *Button) IsOn() bool {
	return b.Input.Read() == rpio.High
}

func (b *Button) Press(ctx context.Context) (<-chan ButtonPress, error) {
	if b.IsPressed() {
		return nil, errors.New("button is already pressed")
	}

	ctx, b.cancel = context.WithTimeout(ctx, b.Timeout)
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
		return ButtonPress{}, errors.New("button is already released")
	}

	if PROD {
		b.Output.Low()
	}
	elapsed := time.Since(b.pendingPress.PressedAt).Round(time.Millisecond)
	log.Printf("button release after %s\n", elapsed)

	b.pendingPress.Elapsed = int(elapsed.Milliseconds())
	b.pendingPress.EndState = b.IsOn()
	b.LastButtonPress = b.pendingPress

	b.doneChan <- b.LastButtonPress
	close(b.doneChan)
	b.cancel()

	return b.LastButtonPress, nil
}

type statusResp struct {
	On        bool         `json:"on"`
	StartedAt time.Time    `json:"started_at"`
	LastPress *ButtonPress `json:"last_press"`
}

func timestamp() time.Time {
	return time.Now().UTC().Round(time.Millisecond)
}

func getStatus() statusResp {
	bp := &button.LastButtonPress
	if bp.Elapsed == 0 {
		bp = nil
	}

	return statusResp{
		On:        button.IsOn(),
		StartedAt: startedAt,
		LastPress: bp,
	}
}

func jsonError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})
	log.Printf("caught error: %s", err.Error())
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "hello world")
}

func handlePress(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	t := r.URL.Query().Get("t")

	if t != "" {
		dur, err := time.ParseDuration(t)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}

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

	if t != "" {
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

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(getStatus())
}

func main() {
	http.HandleFunc("GET /{$}", handleIndex)
	http.HandleFunc("GET /status", handleStatus)
	http.HandleFunc("POST /press", handlePress)
	http.HandleFunc("POST /release", handleRelease)

	http.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		go func() {
			select {
			// case <-ctx.Done():
			// 	fmt.Println("context done", ctx.Err())
			case <-ctx.Done():
				fmt.Println("new context done", ctx.Err())
			}
		}()

		<-ctx.Done()
	})

	log.Println("server online")
	startedAt = timestamp()
	log.Fatal(http.ListenAndServe(":5000", nil))
}
