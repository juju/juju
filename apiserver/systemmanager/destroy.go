// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/names"
	"launchpad.net/tomb"
)

var notify chan string

// DestroySystem will attempt to destroy the system. If the args specify the
// removal of blocks or the destruction of the environments, this method will
// attempt to do so.
func (s *SystemManagerAPI) DestroySystem(args params.DestroySystemArgs) error {
	// Get list of all environments in the system.
	allEnvs, err := s.state.AllEnvironments()
	if err != nil {
		return errors.Trace(err)
	}
	systemEnv, err := s.state.StateServerEnvironment()
	if err != nil {
		return errors.Trace(err)
	}
	systemTag := systemEnv.EnvironTag()

	// If there are alive hosted environments and DestroyEnvironments was not
	// specified, don't bother trying to destroy the system, as it will fail.
	for _, env := range allEnvs {
		environTag := env.EnvironTag()
		if environTag != systemTag && env.Life() == state.Alive && !args.DestroyEnvironments {
			return errors.Errorf("state server environment cannot be destroyed before all other environments are destroyed")
		}
	}

	// If there are blocks, and we aren't being told to ignore them, let the
	// user know.
	blocks, err := s.state.AllBlocksForSystem()
	if err != nil {
		logger.Debugf("Unable to get blocks for system: %s", err)
		if !args.IgnoreBlocks {
			return errors.Trace(err)
		}
	}
	if len(blocks) > 0 {
		if !args.IgnoreBlocks {
			return common.ErrOperationBlocked("found blocks in system environments")
		}

		err := s.state.RemoveAllBlocksForSystem()
		if err != nil {
			return errors.Trace(err)
		}
	}

	hostEnvCount := notDeadCount(allEnvs) - 1
	if hostEnvCount > 0 {

		waiter := newWaitForDeadEnvs(s.state, hostEnvCount)

		if args.DestroyEnvironments {
			for _, env := range allEnvs {
				environTag := env.EnvironTag()
				if environTag != systemTag {
					if err := common.DestroyEnvironment(s.state, environTag); err != nil {
						logger.Errorf("unable to destroy environment %q: %s", env.UUID(), err)
					}
				}
			}
		}
		if err := waiter.Wait(); err != nil {
			return errors.Trace(err)
		}
	}

	return errors.Trace(common.DestroyEnvironment(s.state, systemTag))
}

func newWaitForDeadEnvs(st *state.State, envs int) *waitForDeadEnvs {
	w := &waitForDeadEnvs{
		tomb: tomb.Tomb{},
		st:   st,
		envs: envs,
	}

	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w
}

type waitForDeadEnvs struct {
	tomb tomb.Tomb
	st   *state.State
	envs int
}

// Kill asks the watcher to stop without necessarily
// waiting for it to do so.
func (w *waitForDeadEnvs) Kill(reason error) {
	w.tomb.Kill(reason)
}

// Wait waits for the watcher to exit and returns any
// error encountered when it was running.
func (w *waitForDeadEnvs) Wait() error {
	select {
	case notify <- "waiting for environs to be dead":
	default:
	}
	return w.tomb.Wait()
}

func (w *waitForDeadEnvs) loop() error {
	watcher := w.st.WatchEnvironments()
	defer watcher.Stop()

	for {
		select {
		case uuids := <-watcher.Changes():
			for _, uuid := range uuids {
				if isDead, err := w.isDead(uuid); err != nil {
					return errors.Trace(err)
				} else if isDead {
					w.envs--
				}
			}
			// All environments are dead
			if w.envs == 0 {
				return nil
			}
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

func (w *waitForDeadEnvs) isDead(uuid string) (bool, error) {
	envTag := names.NewEnvironTag(uuid)
	env, err := w.st.GetEnvironment(envTag)
	if err != nil {
		return false, errors.Annotatef(err, "error loading environment %s", envTag.Id())
	}
	return env.Life() == state.Dead, nil
}

func notDeadCount(allEnvs []*state.Environment) int {
	var count int
	for _, env := range allEnvs {
		if env.Life() != state.Dead {
			count++
		}
	}
	return count
}
