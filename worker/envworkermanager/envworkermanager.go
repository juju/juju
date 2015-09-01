// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envworkermanager

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"launchpad.net/tomb"

	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

var (
	logger = loggo.GetLogger("juju.worker.envworkermanager")

	// ripTime is the time to wait after an environment has been set to dead
	// before removing all environment docs.
	ripTime time.Duration
)

func init() {
	ripTime = 24 * time.Hour
}

// NewEnvWorkerManager returns a Worker which manages a worker which
// needs to run on a per environment basis. It takes a function which will
// be called to start a worker for a new environment. This worker
// will be killed when an environment goes away.
func NewEnvWorkerManager(
	st InitialState,
	startEnvWorker func(InitialState, *state.State) (worker.Worker, error),
) worker.Worker {
	m := &envWorkerManager{
		st:             st,
		startEnvWorker: startEnvWorker,
	}
	m.runner = worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant)
	go func() {
		defer m.tomb.Done()
		m.tomb.Kill(m.loop())
	}()

	m.resumeUndertaker()
	return m
}

// InitialState defines the State functionality used by
// envWorkerManager and/or could be useful to startEnvWorker
// funcs. It mainly exists to support testing.
type InitialState interface {
	WatchEnvironments() state.StringsWatcher
	ForEnviron(names.EnvironTag) (*state.State, error)
	GetEnvironment(names.EnvironTag) (*state.Environment, error)
	EnvironUUID() string
	Machine(string) (*state.Machine, error)
	MongoSession() *mgo.Session
	AllEnvironments() ([]*state.Environment, error)
}

type envWorkerManager struct {
	runner         worker.Runner
	tomb           tomb.Tomb
	st             InitialState
	startEnvWorker func(InitialState, *state.State) (worker.Worker, error)
}

// Kill satisfies the Worker interface.
func (m *envWorkerManager) Kill() {
	m.tomb.Kill(nil)
}

// Wait satisfies the Worker interface.
func (m *envWorkerManager) Wait() error {
	return m.tomb.Wait()
}

func (m *envWorkerManager) EnvironIDsFilteredByLife(life state.Life) ([]string, error) {
	envs, err := m.st.AllEnvironments()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var uuids []string
	for _, env := range envs {
		if env.Life() == life {
			uuids = append(uuids, env.UUID())
		}
	}

	return uuids, nil
}

func (m *envWorkerManager) loop() error {
	go func() {
		// When the runner stops, make sure we stop the envWorker as well
		m.tomb.Kill(m.runner.Wait())
	}()
	defer func() {
		// When we return, make sure that we kill
		// the runner and wait for it.
		m.runner.Kill()
		m.tomb.Kill(m.runner.Wait())
	}()
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
		case <-m.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

func (m *envWorkerManager) envHasChanged(uuid string) error {
	envTag := names.NewEnvironTag(uuid)
	var envLife state.Life

	env, err := m.st.GetEnvironment(envTag)
	if errors.IsNotFound(err) {
		envLife = state.Dying
	} else if err != nil {
		return errors.Annotatef(err, "error loading environment %s", envTag.Id())
	} else {
		envLife = env.Life()
	}

	switch envLife {
	case state.Alive:
		err = m.envIsAlive(envTag)
	case state.Dying:
		err = m.envIsDying(envTag)
	case state.Dead:
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

		envRunner, err := m.startEnvWorker(m.st, st)
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

// notify is patched in testing.
var notify = func() {}

func (m *envWorkerManager) envIsDying(envTag names.EnvironTag) error {
	err := m.runner.StopWorker(envTag.Id())
	// envIsDying needs to be idempotent, as it may be called by
	// resumeUndertaker. As such, we need to ignore a stopped worker.
	if err != nil && err != worker.ErrDead {
		return errors.Trace(err)
	}

	err = m.runner.StartWorker("undertaker", func() (worker.Worker, error) {
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

		undertaker := NewUndertaker(st, notify)

		// Close State when the undertaker is done.
		go func() {
			undertaker.Wait()
			closeState()
		}()

		return undertaker, nil
	})

	return errors.Trace(err)
}

var nowToTheSecond = func() time.Time { return time.Now().Round(time.Second).UTC() }

func (m *envWorkerManager) envIsDead(envTag names.EnvironTag) error {
	err := m.runner.StopWorker("undertaker")
	// envIsDead needs to be idempotent, as it may be called by
	// resumeUndertaker. As such we need to ignore a stopped worker.
	if err != nil && err != worker.ErrDead {
		return errors.Trace(err)
	}

	st, err := m.st.ForEnviron(envTag)
	if err != nil {
		return errors.Errorf("failed to open state for environment %s: %v", envTag.Id(), err)
	}

	if st.IsStateServer() {
		// Nothing to do. We don't remove environment docs for a state server
		// environment.
		return st.Close()
	}

	env, err := st.Environment()
	if err != nil {
		return errors.Errorf("could not find dead environment: %v", err)
	}

	// remove all documents for this environment 24hrs after it was destroyed.
	go func() {
		timeDead := nowToTheSecond().Sub(env.TimeOfDeath())
		sleepTime := ripTime - timeDead
		if sleepTime < 0 {
			sleepTime = 0
		}
		time.Sleep(sleepTime)

		if err = st.RemoveAllEnvironDocs(); err != nil {
			logger.Errorf("could not remove all docs for environment %s: %v", envTag.Id(), err)
		}
		if err = st.Close(); err != nil {
			logger.Errorf("error closing state: %v", err)
		}
		notify()
		return
	}()

	return nil
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

// resumeUndertaker is a noOp for Alive environs. If the envworkermanager was
// stopped after an environ was set to dying but before set to dead and all
// environ docs removed, this func will resume the undertaker.
func (m *envWorkerManager) resumeUndertaker() error {
	dyingIDs, err := m.EnvironIDsFilteredByLife(state.Dying)
	if err != nil {
		return errors.Trace(err)
	}
	for _, uuid := range dyingIDs {
		err := m.envIsDying(names.NewEnvironTag(uuid))
		if err != nil {
			return errors.Trace(err)
		}
	}
	deadIDs, err := m.EnvironIDsFilteredByLife(state.Dead)
	if err != nil {
		return errors.Trace(err)
	}
	for _, uuid := range deadIDs {
		err := m.envIsDead(names.NewEnvironTag(uuid))
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
