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

// ripTime is the time to wait after an environment has been set to
// dead, before removing all environment docs.
const ripTime = 24 * time.Hour

// NewUndertaker returns a worker which processes a dying environment.
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
	result, err := u.client.EnvironInfo()
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	envInfo := result.Result

	if envInfo.Life == params.Alive {
		return errors.Errorf("undertaker worker should not be started for an alive environment: %q", envInfo.GlobalName)
	}

	if envInfo.Life == params.Dying {
		// Process the dying environment. This blocks until the environment
		// is dead.
		u.processDyingEnv()
	}

	// If environ is not alive or dying, it must be dead.

	if envInfo.IsSystem {
		// Nothing to do. We don't remove environment docs for a state server
		// environment.
		return nil
	}

	err = u.destroyProviderEnv()
	if err != nil {
		return errors.Trace(err)
	}

	tod := u.clock.Now()
	if envInfo.TimeOfDeath != nil {
		// If TimeOfDeath is not nil, the environment was already dead
		// before the worker was started. So we use the recorded time of
		// death. This may happen if the system is rebooted after an
		// environment is set to dead, but before the environ docs are
		// removed.
		tod = *envInfo.TimeOfDeath
	}

	// Process the dead environment
	return u.processDeadEnv(tod)
}

// Kill is part of the worker.Worker interface.
func (u *undertaker) Kill() {
	u.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (u *undertaker) Wait() error {
	return u.catacomb.Wait()
}

func (u *undertaker) processDyingEnv() error {
	// ProcessDyingEnviron will fail quite a few times before it succeeds as
	// it is being woken up as every machine or service changes. We ignore the
	// error here and rely on the logging inside the ProcessDyingEnviron.
	if err := u.client.ProcessDyingEnviron(); err == nil {
		return nil
	}

	watcher, err := u.client.WatchEnvironResources()
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
				return errors.New("environ resources watcher failed")
			}
			err := u.client.ProcessDyingEnviron()
			if err == nil {
				// ProcessDyingEnviron succeeded. We're done.
				return nil
			}
			// Yes, we ignore the error. See comment above.
		}
	}
}

func (u *undertaker) destroyProviderEnv() error {
	cfg, err := u.client.EnvironConfig()
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

func (u *undertaker) processDeadEnv(timeOfDeath time.Time) error {
	timeDead := u.clock.Now().Sub(timeOfDeath)
	wait := ripTime - timeDead
	if wait < 0 {
		wait = 0
	}

	select {
	case <-u.catacomb.Dying():
		return u.catacomb.ErrDying()
	case <-u.clock.After(wait):
		err := u.client.RemoveEnviron()
		return errors.Annotate(err, "could not remove all docs for dead environment")
	}
}
