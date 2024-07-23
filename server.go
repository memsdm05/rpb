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

	startedAt = timestamp()
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

type Button struct {
	Input   rpio.Pin
	Output  rpio.Pin
	Timeout time.Duration

	pressed bool
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
}

func (b *Button) IsPressed() bool {
	return b.pressed
}

func (b *Button) IsOn() bool {
	return b.Input.Read() == rpio.High
}

func (b *Button) Press(ctx context.Context) (<-chan struct{}, error) {
	if b.IsPressed() {
		return nil, errors.New("button is already pressed")
	}

	b.ctx, b.cancel = context.WithTimeout(ctx, b.Timeout)
	fmt.Println("Button Pressed!")
	b.pressed = true

	go func() {
		<-b.ctx.Done()
		fmt.Println(b.ctx.Err())
		if b.ctx.Err() == context.DeadlineExceeded {
			b.Release()
		}
	}()

	return b.ctx.Done(), nil
}

func (b *Button) Release() error {
	if !b.IsPressed() {
		return errors.New("button is already released")
	}

	fmt.Println("Button Released!")
	b.cancel()
	b.pressed = false
	return nil
}

type statusResp struct {
	On        bool       `json:"on"`
	LastPress *time.Time `json:"last_pressed_at"`
	StartedAt time.Time  `json:"started_at"`
}

func timestamp() time.Time {
	return time.Now().UTC().Round(time.Millisecond)
}

func getStatus() statusResp {
	// lp := &lastPress
	// if lp.IsZero() {
	// 	lp = nil
	// }
	return statusResp{
		On:        button.IsOn(),
		StartedAt: startedAt,
		// LastPress: lp,
	}
}

func jsonError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})

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
		<-done
		http.Redirect(w, r, "/status", http.StatusFound)
	} else {
		w.WriteHeader(http.StatusAccepted)
	}
}

func handleRelease(w http.ResponseWriter, r *http.Request) {
	err := button.Release()
	if err != nil {
		jsonError(w, http.StatusTeapot, err)
		return
	}

	http.Redirect(w, r, "/status", http.StatusFound)
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

	fmt.Println("Ready")
	log.Fatal(http.ListenAndServe(":5000", nil))
}
