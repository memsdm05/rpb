package app

import (
	"context"
	"errors"
	"log"
	"time"
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
	Backend         Backend
	Timeout         time.Duration
	LastButtonPress ButtonPress

	pendingPress   ButtonPress
	on             bool
	pressing       bool
	stateListeners []chan bool
	pressListeners []chan ButtonPress
	cancel         context.CancelFunc
}

func (b *Button) Setup() {
	var err error

	b.Backend.Setup()

	b.stateListeners = make([]chan bool, 0)

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
	b.on = b.Backend.On()
	var wasOn bool
	db.QueryRow("SELECT is_on FROM state ORDER BY rowid DESC LIMIT 1").Scan(&wasOn)

	if b.on == wasOn {
		log.Println("Server probably crashed or experienced a power outage")
	}

	for {
		current := b.Backend.On()
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

			for _, c := range b.stateListeners {
				c <- current
				close(c)
			}
			b.stateListeners = nil

			b.on = current
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (b *Button) IsPressed() bool {
	return b.pressing
}

func (b *Button) IsOn() bool {
	return b.on
}

func (b *Button) Press(source string, ctx context.Context) (<-chan ButtonPress, error) {
	if b.IsPressed() {
		return nil, errors.New("button already pressed")
	}

	ctx, b.cancel = context.WithTimeout(ctx, b.Timeout)
	b.pendingPress = ButtonPress{
		Source:     source,
		PressedAt:  timestamp(),
		StartState: b.IsOn(),
	}
	b.pressing = true
	b.Backend.High()
	log.Printf("Button press by %s\n", source)

	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			log.Println("Encountered timeout")
		}
		b.Release()
	}()

	return b.OnButtonPress(), nil
}

func (b *Button) Release() (ButtonPress, error) {
	if !b.IsPressed() {
		return ButtonPress{}, errors.New("button already released")
	}

	b.pressing = false
	b.Backend.Low()
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

	for _, c := range b.pressListeners {
		c <- b.LastButtonPress
		close(c)
	}
	b.pressListeners = nil
	b.cancel()

	return b.LastButtonPress, nil
}

func (b *Button) OnNewState() <-chan bool {
	c := make(chan bool, 1)
	b.stateListeners = append(b.stateListeners, c)
	return c
}

func (b *Button) OnButtonPress() <-chan ButtonPress {
	c := make(chan ButtonPress, 1)
	b.pressListeners = append(b.pressListeners, c)
	return c
}
