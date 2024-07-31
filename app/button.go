package app

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

type ButtonPress struct {
	Id         int64     `json:"number,omitempty"`
	Source     string    `json:"source"`
	PressedAt  time.Time `json:"pressed_at"`
	Elapsed    float64   `json:"elapsed"`
	StartState bool      `json:"start_state"`
	EndState   bool      `json:"end_state"`
}

type ActualScanner interface {
	Scan(...any) error
}

func ButtonPressFromRow(row ActualScanner) (ButtonPress, error) {
	var (
		err       error
		bp        ButtonPress
		pressedAt string
	)

	err = row.Scan(
		&bp.Id,
		&bp.Source,
		&pressedAt,
		&bp.Elapsed,
		&bp.StartState,
		&bp.EndState,
	)
	if err != nil {
		return bp, err
	}

	bp.PressedAt, err = time.Parse("2006-01-02 15:04:05.999999999-07:00", pressedAt)
	if err != nil {
		return bp, err
	}

	return bp, nil
}

type Button struct {
	Input           rpio.Pin
	Output          rpio.Pin
	Timeout         time.Duration
	Production      bool
	LastButtonPress ButtonPress

	pendingPress ButtonPress
	on           bool
	pressing     bool

	doneChan chan ButtonPress
	cancel   context.CancelFunc
}

func (b *Button) Setup() {
	var err error

	if Config.Production {
		button.Input.Input()
		button.Input.PullUp()

		button.Output.Output()
		button.Output.Low()
	} else {
		b.on = false
	}

	row := db.QueryRow(
		"SELECT id, source, pressed_at, elapsed, start_state, end_state FROM press ORDER BY id DESC LIMIT 1")
	b.LastButtonPress, err = ButtonPressFromRow(row)

	if err != nil {
		log.Printf("Error during last pressed load: %s", err)
		return
	}

	log.Printf("Last press: %+v\n", b.LastButtonPress)

	go b.stateWatcher()
}

func (b *Button) stateWatcher() {
	b.on = b.isOn()
	for {
		current := b.isOn()
		if current != b.on {
			db.Exec(
				`INSERT INTO state (changed_at, is_on, during_press) VALUES (?, ?, ?)`,
				timestamp(), current, b.IsPressed(),
			)

			if current {
				log.Println("State is now on")
			} else {
				log.Println("State is now off")
			}

			b.pressing = current
			b.on = current
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (b *Button) IsPressed() bool {
	return b.pressing
}

func (b *Button) isOn() bool {
	if b.Production {
		return b.Input.Read() == rpio.High
	}
	return true
}

func (b *Button) IsOn() bool {
	return b.on
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
	log.Printf("Button press by %s\n", source)
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

	b.pendingPress.Elapsed = elapsed.Seconds()
	b.pendingPress.EndState = b.IsOn()

	res, err := db.Exec(
		`INSERT INTO press (source, pressed_at, elapsed, start_state, end_state) VALUES (?, ?, ?, ?, ?)`,
		b.pendingPress.Source,
		b.pendingPress.PressedAt,
		b.pendingPress.Elapsed,
		b.pendingPress.StartState,
		b.pendingPress.EndState,
	)
	if err != nil {
		panic(err)
	}
	b.pendingPress.Id, _ = res.LastInsertId()

	log.Printf("Button release #%d after %s\n", b.pendingPress.Id, elapsed)

	b.LastButtonPress = b.pendingPress
	b.doneChan <- b.LastButtonPress
	close(b.doneChan)
	b.cancel()

	return b.LastButtonPress, nil
}