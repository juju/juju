// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package master

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/worker/catacomb"
)

// FlagConfig holds a FlagWorker's dependencies and resources.
type FlagConfig struct {
	Clock    clock.Clock
	Conn     Conn
	Duration time.Duration
}

// Validate returns an error if the config cannot be expected to run a
// FlagWorker.
func (config FlagConfig) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Conn == nil {
		return errors.NotValidf("nil Conn")
	}
	if config.Duration <= 0 {
		return errors.NotValidf("non-positive Duration")
	}
	return nil
}

// FlagWorker implements worker.Worker and engine.Flag. The Check
// method reports whether the supplied Conn corresponds to the
// MongoDB master for the lifetime of the worker.
//
// This worker should be used only rarely, where we must ensure
// that exactly one worker is operating on the MongoDB database,
// where we cannot rely on the database contents for any coordination,
// such as the global clock that underlies the lease management,
// and singular workers.
type FlagWorker struct {
	catacomb catacomb.Catacomb
	config   FlagConfig
	master   bool
}

func NewFlagWorker(config FlagConfig) (*FlagWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating config")
	}
	master, err := config.Conn.IsMaster()
	if err != nil {
		return nil, errors.Trace(err)
	}
	flag := &FlagWorker{
		config: config,
		master: master,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &flag.catacomb,
		Work: func() error {
			return flag.loop(config.Conn.Ping)
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return flag, nil
}

// Kill is part of the worker.Worker interface.
func (flag *FlagWorker) Kill() {
	flag.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (flag *FlagWorker) Wait() error {
	return flag.catacomb.Wait()
}

// Check is part of the engine.Flag interface.
//
// Check returns true if the flag indicates that the configured Identity
// (i.e. this controller) has taken control of the configured Scope (i.e.
// the model we want to manage exclusively).
//
// The validity of this result is tied to the lifetime of the FlagWorker;
// once the worker has stopped, no inferences may be drawn from any Check
// result.
func (flag *FlagWorker) Check() bool {
	return flag.master
}

// run invokes a suitable runFunc, depending on the value of .valid.
func (flag *FlagWorker) loop(ping func() error) error {
	timer := flag.config.Clock.NewTimer(flag.config.Duration)
	defer timer.Stop()
	for {
		select {
		case <-flag.catacomb.Dying():
			return flag.catacomb.ErrDying()
		case <-timer.Chan():
			if err := ping(); err != nil {
				return errors.Annotate(err, "ping failed, flag invalidated")
			}
			timer.Reset(flag.config.Duration)
		}
	}
}
