// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envworkermanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"launchpad.net/tomb"

	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.envworkermanager")

// NewEnvWorkerManager returns a Worker which manages the workers which
// need to run on a per environment basis. It takes a function which will
// be called to start workers for a new environment. These workers
// will be killed when an environment goes away.
func NewEnvWorkerManager(
	st InitialState,
	startEnvWorkers func(InitialState, *state.State) (worker.Runner, error),
) worker.Worker {
	m := &envWorkerManager{
		st:              st,
		startEnvWorkers: startEnvWorkers,
	}
	m.runner = worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant)
	go func() {
		m.tomb.Kill(m.loop())
		m.tomb.Done()
	}()
	return m
}

// InitialState defines the State functionality used by
// envWorkerManager and/or could be useful to startEnvWorkers
// funcs. It mainly exists to support testing.
type InitialState interface {
	WatchEnvironments() state.StringsWatcher
	ForEnviron(names.EnvironTag) (*state.State, error)
	GetEnvironment(names.EnvironTag) (*state.Environment, error)
	EnvironUUID() string
	Machine(string) (*state.Machine, error)
	MongoSession() *mgo.Session
}

type envWorkerManager struct {
	runner          worker.Runner
	tomb            tomb.Tomb
	st              InitialState
	startEnvWorkers func(InitialState, *state.State) (worker.Runner, error)
}

// Kill satisfies the Worker interface.
func (m *envWorkerManager) Kill() {
	m.tomb.Kill(nil)
}

// Wait satisfies the Worker interface.
func (m *envWorkerManager) Wait() error {
	return m.tomb.Wait()
}

func (m *envWorkerManager) loop() error {
	w := m.st.WatchEnvironments()
	defer w.Stop()
	for {
		select {
		case uuids := <-w.Changes():
			// One or more environments have changed.
			for _, uuid := range uuids {
				if err := m.envHasChanged(uuid); err != nil {
					return errors.Trace(err)
				}
			}
		case <-m.runner.Dying():
			// The runner for the environment is dying: wait for it to
			// finish dying and report its error (if any).
			return m.runner.Wait()
		case <-m.tomb.Dying():
			// The envWorkerManager has been asked to die: kill the
			// runner and stop.
			m.runner.Kill()
			return tomb.ErrDying
		}
	}
}

func (m *envWorkerManager) envHasChanged(uuid string) error {
	envTag := names.NewEnvironTag(uuid)
	envAlive, err := m.isEnvAlive(envTag)
	if err != nil {
		return errors.Trace(err)
	}
	if envAlive {
		err = m.envIsAlive(envTag)
	} else {
		err = m.envIsDead(envTag)
	}
	return errors.Trace(err)
}

func (m *envWorkerManager) envIsAlive(envTag names.EnvironTag) error {
	return m.runner.StartWorker(envTag.Id(), func() (worker.Worker, error) {
		st, err := m.st.ForEnviron(envTag)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to open state for environment %s", envTag.Id())
		}
		closeState := func() {
			err := st.Close()
			if err != nil {
				logger.Errorf("error closing state for env %s: %v", envTag.Id(), err)
			}
		}

		envRunner, err := m.startEnvWorkers(m.st, st)
		if err != nil {
			closeState()
			return nil, errors.Trace(err)
		}

		// Close State when the runner for the environment is done.
		go func() {
			envRunner.Wait()
			closeState()
		}()

		return envRunner, nil
	})
}

func (m *envWorkerManager) envIsDead(envTag names.EnvironTag) error {
	err := m.runner.StopWorker(envTag.Id())
	return errors.Trace(err)
}

func (m *envWorkerManager) isEnvAlive(tag names.EnvironTag) (bool, error) {
	env, err := m.st.GetEnvironment(tag)
	if errors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Annotatef(err, "error loading environment %s", tag.Id())
	}
	return env.Life() == state.Alive, nil
}
