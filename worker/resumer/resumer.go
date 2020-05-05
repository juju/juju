// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2/catacomb"
)

var logger = loggo.GetLogger("juju.worker.resumer")

// Facade defines the interface for types capable of resuming
// transactions.
type Facade interface {

	// ResumeTransactions resumes all pending transactions.
	ResumeTransactions() error
}

// Config holds the dependencies and configuration necessary to
// drive a Resumer.
type Config struct {
	Facade   Facade
	Clock    clock.Clock
	Interval time.Duration
}

// Validate returns an error if config cannot be expected to drive
// a Resumer.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Interval <= 0 {
		return errors.NotValidf("non-positive Interval")
	}
	return nil
}

// Resumer is responsible for periodically resuming all pending
// transactions.
type Resumer struct {
	catacomb catacomb.Catacomb
	config   Config
}

// NewResumer returns a new Resumer or an error. If the Resumer is
// not nil, the caller is responsible for stopping it via `Kill()`
// and handling any error returned from `Wait()`.
var NewResumer = func(config Config) (*Resumer, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	rr := &Resumer{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &rr.catacomb,
		Work: rr.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return rr, nil
}

// Kill is part of the worker.Worker interface.
func (rr *Resumer) Kill() {
	rr.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (rr *Resumer) Wait() error {
	return rr.catacomb.Wait()
}

func (rr *Resumer) loop() error {
	var interval time.Duration
	for {
		select {
		case <-rr.catacomb.Dying():
			return rr.catacomb.ErrDying()
		case <-rr.config.Clock.After(interval):
			err := rr.config.Facade.ResumeTransactions()
			if err != nil {
				return errors.Annotate(err, "cannot resume transactions")
			}
		}
		interval = rr.config.Interval
	}
}
