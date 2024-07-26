package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

type ButtonPress struct {
	Source     string    `json:"source"`
	PressedAt  time.Time `json:"pressed_at"`
	Elapsed    float64   `json:"elapsed"`
	StartState bool      `json:"start_state"`
	EndState   bool      `json:"end_state"`
}

type Button struct {
	Input           rpio.Pin
	Output          rpio.Pin
	Timeout         time.Duration
	Production      bool
	LastButtonPress ButtonPress

	pendingPress ButtonPress
	pressing     bool
	doneChan     chan ButtonPress
	cancel       context.CancelFunc
}

func (b *Button) Setup() {
	if !config.Production {
		return
	}

	button.Input.Input()
	button.Input.PullUp()

	button.Output.Output()
	button.Output.Low()
}

func (b *Button) IsPressed() bool {
	return b.pressing
}

func (b *Button) IsOn() bool {
	if b.Production {
		return b.Input.Read() == rpio.High
	}
	return true
}

func (b *Button) Press(source string, ctx context.Context) (<-chan ButtonPress, error) {
	if b.IsPressed() {
		return nil, errors.New("button already pressed")
	}

	ctx, b.cancel = context.WithTimeout(ctx, b.Timeout)
	b.pressing = true
	if b.Production {
		b.Output.High()
	}
	log.Println("Button press")
	b.doneChan = make(chan ButtonPress, 1)
	b.pendingPress = ButtonPress{
		Source:     source,
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
	if b.Production {
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
