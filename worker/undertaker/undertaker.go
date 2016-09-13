// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// Facade covers the parts of the api/undertaker.UndertakerClient that we
// need for the worker. It's more than a little raw, but we'll survive.
type Facade interface {
	ModelInfo() (params.UndertakerModelInfoResult, error)
	WatchModelResources() (watcher.NotifyWatcher, error)
	ProcessDyingModel() error
	RemoveModel() error
	SetStatus(status status.Status, message string, data map[string]interface{}) error
}

// Config holds the resources and configuration necessary to run an
// undertaker worker.
type Config struct {
	Facade  Facade
	Environ environs.Environ
}

// Validate returns an error if the config cannot be expected to drive
// a functional undertaker worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Environ == nil {
		return errors.NotValidf("nil Environ")
	}
	return nil
}

// NewUndertaker returns a worker which processes a dying model.
func NewUndertaker(config Config) (*Undertaker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	u := &Undertaker{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &u.catacomb,
		Work: u.run,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

type Undertaker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is part of the worker.Worker interface.
func (u *Undertaker) Kill() {
	u.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (u *Undertaker) Wait() error {
	return u.catacomb.Wait()
}

func (u *Undertaker) run() error {
	result, err := u.config.Facade.ModelInfo()
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	modelInfo := result.Result

	if modelInfo.Life == params.Alive {
		return errors.Errorf("model still alive")
	}

	if modelInfo.Life == params.Dying {
		// TODO(axw) 2016-04-14 #1570285
		// We should update status with information
		// about the remaining resources here, and
		// also make the worker responsible for
		// checking the emptiness criteria before
		// attempting to remove the model.
		if err := u.setStatus(
			status.Destroying,
			"cleaning up cloud resources",
		); err != nil {
			return errors.Trace(err)
		}
		// Process the dying model. This blocks until the model
		// is dead or the worker is stopped.
		if err := u.processDyingModel(); err != nil {
			return errors.Trace(err)
		}
	}

	// If we get this far, the model must be dead (or *have been*
	// dead, but actually removed by something else since the call).
	if modelInfo.IsSystem {
		// Nothing to do. We don't destroy environ resources or
		// delete model docs for a controller model, because we're
		// running inside that controller and can't safely clean up
		// our own infrastructure. (That'll be the client's job in
		// the end, once we've reported that we've tidied up what we
		// can, by returning nil here, indicating that we've set it
		// to Dead -- implied by processDyingModel succeeding.)
		return nil
	}

	// Now the model is known to be hosted and dead, we can tidy up any
	// provider resources it might have used.
	if err := u.setStatus(
		status.Destroying, "tearing down cloud environment",
	); err != nil {
		return errors.Trace(err)
	}
	if err := u.config.Environ.Destroy(); err != nil {
		return errors.Trace(err)
	}

	// Finally, remove the model.
	if err := u.config.Facade.RemoveModel(); err != nil {
		return errors.Annotate(err, "cannot remove model")
	}
	return nil
}

func (u *Undertaker) setStatus(modelStatus status.Status, message string) error {
	return u.config.Facade.SetStatus(modelStatus, message, nil)
}

func (u *Undertaker) processDyingModel() error {
	watcher, err := u.config.Facade.WatchModelResources()
	if err != nil {
		return errors.Trace(err)
	}
	if err := u.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}
	defer watcher.Kill() // The watcher is not needed once this func returns.

	for {
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		case <-watcher.Changes():
			// TODO(fwereade): this is wrong. If there's a time
			// it's ok to ignore an error, it's when we know
			// exactly what an error is/means. If there's a
			// specific code for "not done yet", *that* is what
			// we should be ignoring. But unknown errors are
			// *unknown*, and we can't just assume that it's
			// safe to continue.
			err := u.config.Facade.ProcessDyingModel()
			if err == nil {
				// ProcessDyingModel succeeded. We're free to
				// destroy any remaining environ resources.
				return nil
			}
			// Yes, we ignore the error. See comment above.
		}
	}
}
