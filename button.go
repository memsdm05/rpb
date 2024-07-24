package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

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
	log.Println("Button press")
	b.doneChan = make(chan ButtonPress, 1)
	b.pendingPress = ButtonPress{
		PressedAt:  timestamp(),
		StartState: b.IsOn(),
	}

	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			log.Println("Encountered timeout")
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
	log.Printf("Button release after %s\n", elapsed)

	b.pendingPress.Elapsed = elapsed.Seconds()
	b.pendingPress.EndState = b.IsOn()
	b.LastButtonPress = b.pendingPress

	b.doneChan <- b.LastButtonPress
	close(b.doneChan)
	b.cancel()

	return b.LastButtonPress, nil
}
