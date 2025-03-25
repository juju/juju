// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pinger

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/errors"
)

// Pinger listens for pings and will call the
// passed action in case of a timeout. This way broken
// or inactive connections can be closed.
type Pinger struct {
	tomb tomb.Tomb
	worker.Worker
	action  func()
	clock   clock.Clock
	timeout time.Duration
	reset   chan struct{}
}

// NewPinger returns a new Pinger instance
// that invokes the given action asynchronously if there
// is more than the given timeout interval between calls
// to its Ping method.
func NewPinger(action func(), clock clock.Clock, timeout time.Duration) *Pinger {
	pt := &Pinger{
		action:  action,
		clock:   clock,
		timeout: timeout,
		reset:   make(chan struct{}),
	}
	pt.tomb.Go(pt.loop)
	return pt
}

// Ping is used by the client heartbeat monitor and resets
// the killer.
func (pt *Pinger) Ping() {
	select {
	case <-pt.tomb.Dying():
	case pt.reset <- struct{}{}:
	}
}

// Kill implements the worker.Worker interface.
func (pt *Pinger) Kill() {
	pt.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (pt *Pinger) Wait() error {
	return pt.tomb.Wait()
}

// loop waits for a reset signal, otherwise it performs
// the initially passed action.
func (pt *Pinger) loop() error {
	for {
		select {
		case <-pt.tomb.Dying():
			return tomb.ErrDying
		case <-pt.reset:
		case <-pt.clock.After(pt.timeout):
			go pt.action()
			return errors.New("ping timeout")
		}
	}
}

// NoopPinger implements the pinger interface, does nothing but waits for heat
// death of the universe.
type NoopPinger struct {
	tomb tomb.Tomb
}

// NewNoopPinger returns a new NoopPinger instance.
func NewNoopPinger() *NoopPinger {
	p := &NoopPinger{}
	p.tomb.Go(func() error {
		<-p.tomb.Dying()
		return tomb.ErrDying
	})
	return p
}

// Ping implements the pinger.Pinger interface.
func (*NoopPinger) Ping() {}

// Kill implements the worker.Worker interface.
func (p *NoopPinger) Kill() {
	p.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (p *NoopPinger) Wait() error {
	return p.tomb.Wait()
}
