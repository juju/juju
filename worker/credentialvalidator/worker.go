// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.api.credentialvalidator")

// ErrValidityChanged indicates that a Worker has bounced because its
// credential validity has changed: either a valid credential became invalid
// or invalid credential became valid.
var ErrValidityChanged = errors.New("cloud credential validity has changed")

// ErrModelCredentialChanged indicates that a Worker has bounced
// because model credential was replaced.
var ErrModelCredentialChanged = errors.New("model credential changed")

// ErrModelDoesNotNeedCredential indicates that a Worker has been uninstalled
// since the model does not have a cloud credential set as the model is
// on the cloud that does not require authentication.
var ErrModelDoesNotNeedCredential = errors.New("model is on the cloud that does not need auth")

// Facade exposes functionality required by a Worker to access and watch
// a cloud credential that a model uses.
type Facade interface {

	// ModelCredential gets model's cloud credential.
	// Models that are on the clouds that do not require auth will return
	// false to signify that credential was not set.
	ModelCredential() (base.StoredCredential, bool, error)

	// WatchCredential gets cloud credential watcher.
	WatchCredential(string) (watcher.NotifyWatcher, error)
}

// Config holds the dependencies and configuration for a Worker.
type Config struct {
	Facade Facade
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	return nil
}

// NewWorker returns a Worker that tracks the validity of the Model's cloud
// credential, as exposed by the Facade.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	mc, err := modelCredential(config.Facade)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w, err := config.Facade.WatchCredential(mc.CloudCredential)
	if err != nil {
		return nil, errors.Trace(err)
	}

	v := &validator{
		validatorFacade: config.Facade,
		credential:      mc,
		watcher:         w,
	}

	err = catacomb.Invoke(catacomb.Plan{
		Site: &v.catacomb,
		Work: v.loop,
		// The watcher needs to be added to the worker's catacomb plan
		// here in order to be controlled by this worker's lifecycle events:
		// for example, to be destroyed when this worker is destroyed, etc.
		// We also add the watcher to the Plan.Init collection to ensure that
		// the worker's Plan.Work method is executed after the watcher
		// is initialised and watcher's changes collection obtains the changes.
		// Watchers that are added using catacomb.Add method
		// miss out on a first call of Worker's Plan.Work method and can, thus,
		// be missing out on an initial change.
		Init: []worker.Worker{w},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return v, nil
}

type validator struct {
	catacomb catacomb.Catacomb

	credential base.StoredCredential
	watcher    watcher.NotifyWatcher

	validatorFacade Facade
}

// Kill is part of the worker.Worker interface.
func (v *validator) Kill() {
	v.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (v *validator) Wait() error {
	return v.catacomb.Wait()
}

func (v *validator) loop() error {
	for {
		select {
		case <-v.catacomb.Dying():
			return v.catacomb.ErrDying()
		case _, ok := <-v.watcher.Changes():
			if !ok {
				return v.catacomb.ErrDying()
			}
			updatedCredential, err := modelCredential(v.validatorFacade)
			if err != nil {
				return errors.Trace(err)
			}
			if v.credential.CloudCredential != updatedCredential.CloudCredential {
				// Model is now using different credential than when this worker was created.
				// TODO (anastasiamac 2018-04-05) - It cannot happen yet
				// but when it can, make sure that this worker still behaves...
				// Also consider - is it appropriate to bounce the worker in that case
				// or just change it's variables to reflect new credential?
				return ErrModelCredentialChanged
			}
			if v.credential.Valid != updatedCredential.Valid {
				return ErrValidityChanged
			}
		}
	}
}

func modelCredential(v Facade) (base.StoredCredential, error) {
	mc, exists, err := v.ModelCredential()
	if err != nil {
		return base.StoredCredential{}, errors.Trace(err)
	}
	if !exists {
		logger.Warningf("model credential is not set for the model")
		return base.StoredCredential{}, ErrModelDoesNotNeedCredential
	}
	return mc, nil
}
