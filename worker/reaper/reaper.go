// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reaper

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	apireaper "github.com/juju/juju/api/reaper"
	"github.com/juju/juju/apiserver/params"

	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.reaper")

// NewReaper returns a worker which tears down all resources for a dying environment.
func NewReaper(client apireaper.ReaperClient) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		result, err := client.EnvironInfo()
		if err != nil {
			return errors.Trace(err)
		}
		if result.Error != nil {
			return errors.Trace(result.Error)
		}
		envInfo := result.Result

		// TODO(waigani)
		if envInfo.Life != params.Alive {
			return errors.Errorf("reaper worker should not be started for an alive environment: %q", envInfo.GlobalName)
		}

		if envInfo.Life == params.Dying {
			// Process the alive environment. This blocks until the environment
			// is dying.
			processDyingEnv(client, clock, stopCh)
		}

		// If environ is not alive or dying, it must be dead.

		if envInfo.IsSystem {
			// Nothing to do. We don't remove environment docs for a state server
			// environment.
			return nil
		}

		tod := clock.Now()
		if envInfo.TimeOfDeath != nil {
			// If TimeOfDeath is not nil, the environment was already dead
			// before the worker was started. So we use the recorded time of
			// death. This may happen if the system is rebooted after an
			// environment is set to dead, but before the environ docs are
			// removed.
			tod = *envInfo.TimeOfDeath
		}

		// Process the dead environment
		return processDeadEnv(client, clock, tod, stopCh)
	}
	return worker.NewSimpleWorker(f)
}

func ReapEnv(client apireaper.ReaperClient, stopCh <-chan struct{}) error {
	// PeapEnviron waits until the environment is dead before it reaps it.
	// error here and rely on the logging inside the ProcessDyingEnviron.
	if err := client.ReapEnviron(); err == nil {
		return nil
	}
	watcher, err := client.WatchEnvironResources()
	if err != nil {
		return errors.Trace(err)
	}
	defer watcher.Stop()
	for {
		select {
		case <-clock.After(reaperPeriod):
		case _, ok := <-watcher.Changes():
			if !ok {
				return watcher.Err()
			}
			err := client.ProcessDyingEnviron()
			if err != nil {
				logger.Warningf("failed to process dying environment: %v - will retry later", err)
				// Yes, we ignore the error. See comment above.
				continue
			}
			return nil
		case <-stopCh:
			return tomb.ErrDying
		}
	}
}

func teardownDyingEnvironment() {

}

// func areAllHostedEnvsDead(st *state.State) (bool, error) {
// 	allEnvs, err := st.AllEnvironments()
// 	if err != nil {
// 		return false, errors.Trace(err)
// 	}

// 	var notDeadCount int
// 	for _, env := range allEnvs {
// 		if env.Life() != state.Dead && env.ServerUUID() != env.UUID() {
// 			notDeadCount++
// 		}

// 		if notDeadCount == 0 {
// 			return false, nil
// 		}
// 	}

// 	return true, nil
// }

// type environDestroyer struct {
// 	tomb      tomb.Tomb
// 	st        *state.State
// 	systemTag names.EnvironTag

// 	// livingEnvirons is a chan of host environment tags.
// 	livingEnvirons chan names.EnvironTag
// }

// func processDyingController(client apireaper.ReaperClient, stopCh <-chan struct{}) error {
// 	// ProcessDyingEnviron will fail quite a few times before it succeeds as
// 	// it is being woken up as every machine or service changes. We ignore the
// 	// error here and rely on the logging inside the ProcessDyingEnviron.
// 	if err := client.ProcessDyingEnviron(); err == nil {
// 		return nil
// 	}
// 	watcher, err := client.WatchEnvironResources()
// 	if err != nil {
// 		return errors.Trace(err)
// 	}
// 	defer watcher.Stop()

// 	for {
// 		select {
// 		case _, ok := <-watcher.Changes():
// 			if !ok {
// 				return watcher.Err()
// 			}

// 			err := client.ProcessDyingEnviron()
// 			if err != nil {
// 				// Yes, we ignore the error. See comment above.
// 				continue
// 			}
// 			return nil
// 		case <-stopCh:
// 			return tomb.ErrDying
// 		}
// 	}
// }

// func newEnvironDestroyer(st *state.State, systemTag names.EnvironTag) *environDestroyer {

// 	dest := &environDestroyer{
// 		st:             st,
// 		systemTag:      systemTag,
// 		livingEnvirons: make(chan names.EnvironTag),
// 	}

// 	go func() {
// 		defer e.tomb.Done()
// 		e.tomb.Kill(e.loop())
// 	}()

// 	return dest
// }

// // Kill asks the watcher to stop without necessarily
// // waiting for it to do so.
// func (w *environDestroyer) Kill() {
// 	close(w.livingEnvirons)
// 	w.tomb.Kill(nil)
// }

// // Wait waits for the watcher to exit and returns any
// // error encountered when it was running.
// func (w *environDestroyer) Wait() error {
// 	return w.tomb.Wait()
// }

// func (w *environDestroyer) loop() error {
// 	envWatcher := w.st.WatchEnvironments()
// 	defer watcher.Stop(envWatcher, &w.tomb)

// 	// There is a chance that the hosted environments have died before we
// 	// started waiting. So we check here before we wait.
// 	if allDead, err := areAllHostedEnvsDead(w.st); err != nil {
// 		return errors.Trace(err)
// 	} else if allDead {
// 		return nil
// 	}

// 	// undeadEnvs is a slice of environ UUIDs that errored when being destroyed.
// 	var undeadEnvs []string

// 	for {
// 		select {
// 		case <-w.tomb.Dying():
// 			return tomb.ErrDying
// 		case environTag := <-w.livingEnvirons:
// 			if environTag != w.systemTag {
// 				if err := common.DestroyEnvironment(w.st, environTag, false); err != nil {
// 					logger.Errorf("unable to destroy environment %q: %s", environTag.Id(), err)
// 					undeadEnvs = append(undeadEnvs, environTag.Id())
// 				}
// 			}

// 		case uuids, ok := <-envWatcher.Changes():
// 			if !ok {
// 				// The tomb is already killed with the correct error
// 				// at this point, so just return.
// 				return nil
// 			}
// 			for _, uuid := range uuids {
// 				if isDead, err := w.isDead(uuid); err != nil {
// 					return errors.Trace(err)
// 				} else if isDead {
// 					// Was this the last environ do die?
// 					if allDead, err := areAllHostedEnvsDead(w.st); err != nil {
// 						return errors.Trace(err)
// 					} else if allDead {
// 						return nil
// 					}
// 				}
// 			}
// 		}
// 	}
// }

// func (w *environDestroyer) isDead(uuid string) (bool, error) {
// 	envTag := names.NewEnvironTag(uuid)
// 	env, err := w.st.GetEnvironment(envTag)
// 	if err != nil {
// 		return false, errors.Annotatef(err, "error loading environment %s", envTag.Id())
// 	}
// 	return env.Life() == state.Dead, nil
// }

// 	machines, err := st.AllMachines()
// 	if err != nil {
// 		return errors.Trace(err)
// 	}

// 	// We must destroy instances server-side to support JES (Juju Environment
// 	// Server), as there's no CLI to fall back on. In that case, we only ever
// 	// destroy non-state machines; we leave destroying state servers in non-
// 	// hosted environments to the CLI, as otherwise the API server may get cut
// 	// off.
// 	if err := destroyNonManagerMachines(st, machines); err != nil {
// 		return errors.Trace(err)
// 	}

// // destroyNonManagerMachines directly destroys all non-manager, non-manual
// // machine instances.
// func destroyNonManagerMachines(st *state.State, machines []*state.Machine) error {
// 	var ids []string
// 	for _, m := range machines {
// 		if m.IsManager() {
// 			continue
// 		}
// 		if _, isContainer := m.ParentId(); isContainer {
// 			continue
// 		}
// 		manual, err := m.IsManual()
// 		if err != nil {
// 			return err
// 		} else if manual {
// 			continue
// 		}
// 		ids = append(ids, m.Id())
// 	}
// 	if len(ids) == 0 {
// 		return nil
// 	}

// 	return DestroyMachines(st, true, ids...)
// }
