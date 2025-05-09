// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// ErrValidityChanged indicates that a Worker has bounced because its
// credential validity has changed: either a valid credential became invalid
// or invalid credential became valid.
var ErrValidityChanged = errors.New("cloud credential validity has changed")

// ErrModelCredentialChanged indicates that a Worker has bounced because its
// model's cloud credential has changed.
var ErrModelCredentialChanged = errors.New("model cloud credential has changed")

// Facade exposes functionality required by a Worker to access and watch
// a cloud credential that a model uses.
type Facade interface {
	// ModelCredential gets model's cloud credential.
	// Models that are on the clouds that do not require auth will return
	// false to signify that credential was not set.
	ModelCredential(context.Context) (base.StoredCredential, bool, error)

	// WatchModelCredential gets model's cloud credential watcher.
	WatchModelCredential(context.Context) (watcher.NotifyWatcher, error)
}

// Config holds the dependencies and configuration for a Worker.
type Config struct {
	Facade Facade
	Logger logger.Logger
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewWorker returns a Worker that tracks the validity of the Model's cloud
// credential, as exposed by the Facade.
func NewWorker(ctx context.Context, config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	mc, err := modelCredential(ctx, config.Facade)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// This worker needs to monitor both the changes to the credential content that
	// this model uses as well as what credential the model uses.
	// It needs to be restarted if there is a change in either.
	mcw, err := config.Facade.WatchModelCredential(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	v := &validator{
		validatorFacade:        config.Facade,
		logger:                 config.Logger,
		credential:             mc,
		modelCredentialWatcher: mcw,
	}

	// The watcher needs to be added to the worker's catacomb plan
	// here in order to be controlled by this worker's lifecycle events:
	// for example, to be destroyed when this worker is destroyed, etc.
	// We also add the watcher to the Plan.Init collection to ensure that
	// the worker's Plan.Work method is executed after the watcher
	// is initialised and watcher's changes collection obtains the changes.
	// Watchers that are added using catacomb.Add method
	// miss out on a first call of Worker's Plan.Work method and can, thus,
	// be missing out on an initial change.
	plan := catacomb.Plan{
		Name: "credential-validator",
		Site: &v.catacomb,
		Work: v.loop,
		Init: []worker.Worker{v.modelCredentialWatcher},
	}

	if err := catacomb.Invoke(plan); err != nil {
		return nil, errors.Trace(err)
	}
	return v, nil
}

type validator struct {
	catacomb        catacomb.Catacomb
	validatorFacade Facade
	logger          logger.Logger

	modelCredentialWatcher watcher.NotifyWatcher

	credential base.StoredCredential
}

// Kill is part of the worker.Worker interface.
func (v *validator) Kill() {
	v.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (v *validator) Wait() error {
	return v.catacomb.Wait()
}

// Check is part of the util.Flag interface.
func (v *validator) Check() bool {
	return v.credential.Valid
}

func (v *validator) loop() error {
	ctx, cancel := v.scopedContext()
	defer cancel()

	for {
		select {
		case <-v.catacomb.Dying():
			return v.catacomb.ErrDying()
		case _, ok := <-v.modelCredentialWatcher.Changes():
			if !ok {
				return v.catacomb.ErrDying()
			}
			updatedCredential, err := modelCredential(ctx, v.validatorFacade)
			if err != nil {
				return errors.Trace(err)
			}
			if v.credential.CloudCredential != updatedCredential.CloudCredential {
				return ErrModelCredentialChanged
			}
			if v.credential.Valid != updatedCredential.Valid {
				return ErrValidityChanged
			}
		}
	}
}

func (v *validator) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(v.catacomb.Context(context.Background()))
}

func modelCredential(ctx context.Context, v Facade) (base.StoredCredential, error) {
	mc, _, err := v.ModelCredential(ctx)
	if err != nil {
		return base.StoredCredential{}, errors.Trace(err)
	}
	return mc, nil
}
