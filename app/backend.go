package app

import (
	"time"

	"github.com/stianeikeland/go-rpio/v4"
)

type Backend interface {
	Setup()
	High()
	Low()
	On() bool
}

type RpioBackend struct {
	Input  rpio.Pin
	Output rpio.Pin
}

func (r *RpioBackend) Setup() {
	r.Input.Input()
	r.Input.PullUp()

	r.Output.Output()
	r.Output.Low()
}

func (r *RpioBackend) High() {
	r.Output.High()
}

func (r *RpioBackend) Low() {
	r.Output.Low()
}

func (d *RpioBackend) On() bool {
	return d.Input.Read() == rpio.High
}

type DummyBackend struct {
	TurnOffTime time.Duration
	lastPress   time.Time
	on          bool
}

func (d *DummyBackend) Setup() {}
func (d *DummyBackend) High()  {}

func (d *DummyBackend) Low() {
	d.on = !d.on
}

func (d *DummyBackend) On() bool {
	return d.on
}
