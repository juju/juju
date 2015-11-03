// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// DestroySystem will attempt to destroy the system. If the args specify the
// removal of blocks or the destruction of the environments, this method will
// attempt to do so.
func (s *SystemManagerAPI) DestroySystem(args params.DestroySystemArgs) error {
	systemEnv, err := s.state.StateServerEnvironment()
	if err != nil {
		return errors.Trace(err)
	}
	systemTag := systemEnv.EnvironTag()

	if err = s.ensureNotBlocked(args); err != nil {
		return errors.Trace(err)
	}

	// If we are destroying environments, we need to tolerate living
	// environments but set the system to dying to prevent new environments
	// sneaking in. If we are not destroying hosted environments, this will
	// fail if any hosted environments are found.
	if err = common.DestroyEnvironment(s.state, systemTag, args.DestroyEnvironments); err != nil {
		if state.IsHasHostedEnvironsError(err) {
			return errors.New("state server environment cannot be destroyed before all other environments are destroyed")
		}
		return errors.Trace(err)
	}

	// Now we can be sure that no new environments will be added. But any of
	// the environments may already be dying and die at any moment. So we need
	// to watch and wait for all envs to be dead while destroying any
	// living environments we've found.
	destroyer := newEnvironDestroyer(s.state, systemTag)

	var allEnvs []*state.Environment
	allEnvs, err = s.state.AllEnvironments()
	if err != nil {
		return errors.Trace(err)
	}

	for _, env := range allEnvs {
		environTag := env.EnvironTag()
		if environTag != systemTag {
			destroyer.livingEnvirons <- environTag
		}
	}

	return destroyer.Wait()
}

func areAllHostedEnvsDead(st *state.State) (bool, error) {
	allEnvs, err := st.AllEnvironments()
	if err != nil {
		return false, errors.Trace(err)
	}

	var notDeadCount int
	for _, env := range allEnvs {
		if env.Life() != state.Dead && env.ServerUUID() != env.UUID() {
			notDeadCount++
		}

		if notDeadCount == 0 {
			return false, nil
		}
	}

	return true, nil
}

type environDestroyer struct {
	tomb      tomb.Tomb
	st        *state.State
	systemTag names.EnvironTag

	// livingEnvirons is a chan of host environment tags.
	livingEnvirons chan names.EnvironTag
}

func newEnvironDestroyer(st *state.State, systemTag names.EnvironTag) *environDestroyer {

	dest := &environDestroyer{
		st:             st,
		systemTag:      systemTag,
		livingEnvirons: make(chan names.EnvironTag),
	}

	go func() {
		defer e.tomb.Done()
		e.tomb.Kill(e.loop())
	}()

	return dest
}

// Kill asks the watcher to stop without necessarily
// waiting for it to do so.
func (w *environDestroyer) Kill() {
	close(w.livingEnvirons)
	w.tomb.Kill(nil)
}

// Wait waits for the watcher to exit and returns any
// error encountered when it was running.
func (w *environDestroyer) Wait() error {
	return w.tomb.Wait()
}

func (w *environDestroyer) loop() error {
	envWatcher := w.st.WatchEnvironments()
	defer watcher.Stop(envWatcher, &w.tomb)

	// There is a chance that the hosted environments have died before we
	// started waiting. So we check here before we wait.
	if allDead, err := areAllHostedEnvsDead(w.st); err != nil {
		return errors.Trace(err)
	} else if allDead {
		return nil
	}

	// undeadEnvs is a slice of environ UUIDs that errored when being destroyed.
	var undeadEnvs []string

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case environTag := <-w.livingEnvirons:
			if environTag != w.systemTag {
				if err := common.DestroyEnvironment(w.st, environTag, false); err != nil {
					logger.Errorf("unable to destroy environment %q: %s", environTag.Id(), err)
					undeadEnvs = append(undeadEnvs, environTag.Id())
				}
			}

		case uuids, ok := <-envWatcher.Changes():
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
			for _, uuid := range uuids {
				if isDead, err := w.isDead(uuid); err != nil {
					return errors.Trace(err)
				} else if isDead {
					// Was this the last environ do die?
					if allDead, err := areAllHostedEnvsDead(w.st); err != nil {
						return errors.Trace(err)
					} else if allDead {
						return nil
					}
				}
			}
		}
	}
}

func (w *environDestroyer) isDead(uuid string) (bool, error) {
	envTag := names.NewEnvironTag(uuid)
	env, err := w.st.GetEnvironment(envTag)
	if err != nil {
		return false, errors.Annotatef(err, "error loading environment %s", envTag.Id())
	}
	return env.Life() == state.Dead, nil
}

func (s *SystemManagerAPI) ensureNotBlocked(args params.DestroySystemArgs) error {
	if args.IgnoreBlocks {
		err := s.state.RemoveAllBlocksForSystem()
		if err != nil {
			return errors.Trace(err)
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

	if len(blocks) > 0 && !args.IgnoreBlocks {
		return common.ErrOperationBlocked("found blocks in system environments")
	}
	return nil
}
