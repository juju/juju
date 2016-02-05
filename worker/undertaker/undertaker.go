// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	uc "github.com/juju/utils/clock"

	apiundertaker "github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.undertaker")

// ripTime is the time to wait after an model has been set to
// dead, before removing all model docs.
const ripTime = 24 * time.Hour

// NewUndertaker returns a worker which processes a dying model.
func NewUndertaker(client apiundertaker.UndertakerClient, clock uc.Clock) (worker.Worker, error) {
	u := &undertaker{
		client: client,
		clock:  clock,
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

type undertaker struct {
	catacomb catacomb.Catacomb
	client   apiundertaker.UndertakerClient
	clock    uc.Clock
}

func (u *undertaker) run() error {
	result, err := u.client.ModelInfo()
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	modelInfo := result.Result

	if modelInfo.Life == params.Alive {
		return errors.Errorf("undertaker worker should not be started for an alive model: %q", modelInfo.GlobalName)
	}

	if modelInfo.Life == params.Dying {
		// Process the dying model. This blocks until the model
		// is dead.
		u.processDyingModel()
	}

	// If model is not alive or dying, it must be dead.

	if modelInfo.IsSystem {
		// Nothing to do. We don't remove model docs for a controller
		// model.
		return nil
	}

	err = u.destroyProviderModel()
	if err != nil {
		return errors.Trace(err)
	}

	tod := u.clock.Now()
	if modelInfo.TimeOfDeath != nil {
		// If TimeOfDeath is not nil, the model was already dead
		// before the worker was started. So we use the recorded time of
		// death. This may happen if the system is rebooted after an
		// model is set to dead, but before the model docs are
		// removed.
		tod = *modelInfo.TimeOfDeath
	}

	// Process the dead model
	return u.processDeadModel(tod)
}

// Kill is part of the worker.Worker interface.
func (u *undertaker) Kill() {
	u.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (u *undertaker) Wait() error {
	return u.catacomb.Wait()
}

func (u *undertaker) processDyingModel() error {
	// ProcessDyingModel will fail quite a few times before it succeeds as
	// it is being woken up as every machine or service changes. We ignore the
	// error here and rely on the logging inside the ProcessDyingModel.
	if err := u.client.ProcessDyingModel(); err == nil {
		return nil
	}

	watcher, err := u.client.WatchModelResources()
	if err != nil {
		return errors.Trace(err)
	}
	defer watcher.Kill() // The watcher is not needed once this func returns.
	if err := u.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		case _, ok := <-watcher.Changes():
			if !ok {
				return errors.New("model resources watcher failed")
			}
			err := u.client.ProcessDyingModel()
			if err == nil {
				// ProcessDyingModel succeeded. We're done.
				return nil
			}
			// Yes, we ignore the error. See comment above.
		}
	}
}

func (u *undertaker) destroyProviderModel() error {
	cfg, err := u.client.ModelConfig()
	if err != nil {
		return errors.Trace(err)
	}
	env, err := environs.New(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	err = env.Destroy()
	return errors.Trace(err)
}

func (u *undertaker) processDeadModel(timeOfDeath time.Time) error {
	timeDead := u.clock.Now().Sub(timeOfDeath)
	wait := ripTime - timeDead
	if wait < 0 {
		wait = 0
	}

	select {
	case <-u.catacomb.Dying():
		return u.catacomb.ErrDying()
	case <-u.clock.After(wait):
		err := u.client.RemoveModel()
		return errors.Annotate(err, "could not remove all docs for dead model")
	}
}
