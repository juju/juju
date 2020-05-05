// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/core/lease"
)

// Facade exposes the capabilities required by a FlagWorker.
type Facade interface {
	Claim(duration time.Duration) error
	Wait() error
}

// FlagConfig holds a FlagWorker's dependencies and resources.
type FlagConfig struct {
	Clock    clock.Clock
	Facade   Facade
	Duration time.Duration
}

// Validate returns an error if the config cannot be expected to run a
// FlagWorker.
func (config FlagConfig) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Duration <= 0 {
		return errors.NotValidf("non-positive Duration")
	}
	return nil
}

// ErrRefresh indicates that the flag's Check result is no longer valid,
// and a new FlagWorker must be started to get a valid result.
var ErrRefresh = errors.New("model responsibility unclear, please retry")

// FlagWorker implements worker.Worker and util.Flag, representing
// controller ownership of a model, such that the Flag's validity is tied
// to the Worker's lifetime.
type FlagWorker struct {
	catacomb catacomb.Catacomb
	config   FlagConfig
	valid    bool
}

func NewFlagWorker(config FlagConfig) (*FlagWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	valid, err := claim(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	flag := &FlagWorker{
		config: config,
		valid:  valid,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &flag.catacomb,
		Work: flag.run,
	})
	if err != nil {
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

// Check is part of the util.Flag interface.
//
// Check returns true if the flag indicates that the configured Identity
// (i.e. this controller) has taken control of the configured Scope (i.e.
// the model we want to manage exclusively).
//
// The validity of this result is tied to the lifetime of the FlagWorker;
// once the worker has stopped, no inferences may be drawn from any Check
// result.
func (flag *FlagWorker) Check() bool {
	return flag.valid
}

// run invokes a suitable runFunc, depending on the value of .valid.
func (flag *FlagWorker) run() error {
	runFunc := waitVacant
	if flag.valid {
		runFunc = keepOccupied
	}
	err := runFunc(flag.config, flag.catacomb.Dying())
	return errors.Trace(err)
}

// keepOccupied is a runFunc that tries to keep a flag valid.
func keepOccupied(config FlagConfig, abort <-chan struct{}) error {
	for {
		select {
		case <-abort:
			return nil
		case <-sleep(config):
			success, err := claim(config)
			if err != nil {
				return errors.Trace(err)
			}
			if !success {
				return ErrRefresh
			}
		}
	}
}

// claim claims model ownership on behalf of a controller, and returns
// true if the attempt succeeded.
func claim(config FlagConfig) (bool, error) {
	err := config.Facade.Claim(config.Duration)
	cause := errors.Cause(err)
	switch cause {
	case nil:
		return true, nil
	case lease.ErrClaimDenied:
		return false, nil
	}
	return false, errors.Trace(err)
}

// sleep waits for half the duration of a (presumed) earlier successful claim.
func sleep(config FlagConfig) <-chan time.Time {
	return config.Clock.After(config.Duration / 2)
}

// wait is a runFunc that ignores its abort chan and always returns an error;
// either because of a failed api call, or a successful one, which indicates
// that no lease is held; hence, that the worker should be bounced.
func waitVacant(config FlagConfig, _ <-chan struct{}) error {
	if err := config.Facade.Wait(); err != nil {
		return errors.Trace(err)
	}
	return ErrRefresh
}
