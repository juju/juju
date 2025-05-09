// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/model"
)

// FlagConfig holds a FlagWorker's dependencies and resources.
type FlagConfig struct {
	LeaseManager lease.Manager
	ModelUUID    model.UUID
	Claimant     names.Tag
	Entity       names.Tag
	Clock        clock.Clock
	Duration     time.Duration
}

// Validate returns an error if the config cannot be expected to run a
// FlagWorker.
func (config FlagConfig) Validate() error {
	if config.LeaseManager == nil {
		return errors.NotValidf("nil LeaseManager")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.Claimant == nil {
		return errors.NotValidf("nil Claimant")
	}
	if entity := config.Entity; entity == nil || entity.Id() == "" {
		return errors.NotValidf("empty Entity")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Duration <= 0 {
		return errors.NotValidf("non-positive Duration")
	}
	return nil
}

// ErrRefresh indicates that the flag's Check result is no longer valid,
// and a new FlagWorker must be started to get a valid result.
const ErrRefresh = errors.ConstError("model responsibility unclear, please retry")

// FlagWorker implements worker.Worker and util.Flag, representing
// controller ownership of a model, such that the Flag's validity is tied
// to the Worker's lifetime.
type FlagWorker struct {
	catacomb catacomb.Catacomb
	config   FlagConfig
	valid    bool

	claimer lease.Claimer
}

// NewFlagWorker returns a FlagWorker that claims and maintains ownership
// of a model, as long as the worker is running.
func NewFlagWorker(ctx context.Context, config FlagConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	claimer, err := config.LeaseManager.Claimer(lease.SingularControllerNamespace, config.ModelUUID.String())
	if err != nil {
		return nil, errors.Trace(err)
	}

	flag := &FlagWorker{
		config:  config,
		claimer: claimer,
	}

	valid, err := flag.claim(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	flag.valid = valid

	err = catacomb.Invoke(catacomb.Plan{
		Name: "singular",
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
	ctx, cancel := flag.scopedContext()
	defer cancel()

	runFunc := flag.waitVacant
	if flag.valid {
		runFunc = flag.keepOccupied
	}
	return errors.Trace(runFunc(ctx))
}

func (flag *FlagWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(flag.catacomb.Context(context.Background()))
}

// keepOccupied is a runFunc that tries to keep a flag valid.
func (flag *FlagWorker) keepOccupied(ctx context.Context) error {
	for {
		select {
		case <-flag.catacomb.Dying():
			return flag.catacomb.ErrDying()

		case <-flag.config.Clock.After(flag.config.Duration / 2):
			success, err := flag.claim(ctx)
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
func (flag *FlagWorker) claim(ctx context.Context) (bool, error) {
	err := flag.claimer.Claim(flag.config.Entity.Id(), flag.config.Claimant.String(), flag.config.Duration)
	if errors.Is(err, lease.ErrClaimDenied) {
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

// wait is a runFunc that ignores its abort chan and always returns an error;
// either because of a failed api call, or a successful one, which indicates
// that no lease is held; hence, that the worker should be bounced.
func (flag *FlagWorker) waitVacant(ctx context.Context) error {
	// We don't care about if the claim has been started, so just create a
	// buffered channel to avoid blocking the claimer.
	started := make(chan struct{}, 1)
	if err := flag.claimer.WaitUntilExpired(ctx, flag.config.Entity.Id(), started); err != nil {
		return errors.Trace(err)
	}
	return ErrRefresh
}
