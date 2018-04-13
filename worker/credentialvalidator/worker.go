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

// Predicate defines a predicate.
type Predicate func(base.StoredCredential) bool

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

// IsValid returns true when the given credential is valid.
func IsValid(c base.StoredCredential) bool {
	return c.Valid
}

// Config holds the dependencies and configuration for a Worker.
type Config struct {
	Facade Facade
	Check  Predicate
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Check == nil {
		return errors.NotValidf("nil Check")
	}
	return nil
}

// New returns a Worker that tracks the result of the configured
// Check on the Model's cloud credential, as exposed by the Facade.
func New(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	modelCredential, exists, err := config.Facade.ModelCredential()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !exists {
		return nil, ErrModelDoesNotNeedCredential
	}
	watcher, err := config.Facade.WatchCredential(modelCredential.CloudCredential)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config:          config,
		watcher:         watcher,
		modelCredential: modelCredential,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Init: []worker.Worker{watcher},
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker implements worker.Worker and util.Flag, and exits
// with ErrChanged whenever the result of its configured Check of
// the Model's cloud credential changes.
type Worker struct {
	catacomb        catacomb.Catacomb
	config          Config
	watcher         watcher.NotifyWatcher
	modelCredential base.StoredCredential
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Check is part of the util.Flag interface.
func (w *Worker) Check() bool {
	return w.config.Check(w.modelCredential)
}

func (w *Worker) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-w.watcher.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			mc, exists, err := w.config.Facade.ModelCredential()
			if err != nil {
				return errors.Trace(err)
			}
			if !exists {
				// Really should not happen in practice since the uninstall
				// should have occurred back in the constructor.
				// If the credential came back as not set here, it could be a bigger problem:
				// the model must have had a credential at some stage and now it's removed.
				logger.Warningf("model credential was unexpectedly unset for the model that resides on the cloud that requires auth")
				return ErrModelDoesNotNeedCredential
			}
			if mc.CloudCredential != w.modelCredential.CloudCredential {
				// Model is now using different credential than when this worker was created.
				// TODO (anastasiamac 2018-04-05) - It cannot happen yet
				// but when it can, make sure that this worker still behaves...
				// Also consider - is it appropriate to bounce the worker in that case
				// or just change it's variables to reflect new credential?
				return ErrModelCredentialChanged
			}
			if w.Check() != w.config.Check(mc) {
				return ErrValidityChanged
			}
		}
	}
}
