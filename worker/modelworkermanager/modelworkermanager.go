// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

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

var logger = loggo.GetLogger("juju.worker.modelworkermanager")

type modelWorkersCreator func(InitialState, *state.State) (worker.Worker, error)

// NewModelWorkerManager returns a Worker which manages a worker which
// needs to run on a per model basis. It takes a function which will
// be called to start a worker for a new model. This worker
// will be killed when an model goes away.
func NewModelWorkerManager(
	st InitialState,
	startModelWorker modelWorkersCreator,
	dyingModelWorker modelWorkersCreator,
	delay time.Duration,
) worker.Worker {
	m := &modelWorkerManager{
		st:               st,
		startModelWorker: startModelWorker,
		dyingModelWorker: dyingModelWorker,
	}
	m.runner = worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant, delay)
	go func() {
		defer m.tomb.Done()
		m.tomb.Kill(m.loop())
	}()
	return m
}

// InitialState defines the State functionality used by
// envWorkerManager and/or could be useful to startEnvWorker
// funcs. It mainly exists to support testing.
type InitialState interface {
	WatchModels() state.StringsWatcher
	ForModel(names.ModelTag) (*state.State, error)
	GetModel(names.ModelTag) (*state.Model, error)
	ModelUUID() string
	Machine(string) (*state.Machine, error)
	MongoSession() *mgo.Session
}

type modelWorkerManager struct {
	runner           worker.Runner
	tomb             tomb.Tomb
	st               InitialState
	startModelWorker modelWorkersCreator
	dyingModelWorker modelWorkersCreator
}

// Kill satisfies the Worker interface.
func (m *modelWorkerManager) Kill() {
	m.tomb.Kill(nil)
}

// Wait satisfies the Worker interface.
func (m *modelWorkerManager) Wait() error {
	return m.tomb.Wait()
}

func (m *modelWorkerManager) loop() error {
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
	w := m.st.WatchModels()
	defer w.Stop()
	for {
		select {
		case uuids := <-w.Changes():
			// One or more models have changed.
			for _, uuid := range uuids {
				if err := m.modelHasChanged(uuid); err != nil {
					return errors.Trace(err)
				}
			}
		case <-m.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

func (m *modelWorkerManager) modelHasChanged(uuid string) error {
	modelTag := names.NewModelTag(uuid)
	env, err := m.st.GetModel(modelTag)
	if errors.IsNotFound(err) {
		return m.modelNotFound(modelTag)
	} else if err != nil {
		return errors.Annotatef(err, "error loading model %s", modelTag.Id())
	}

	switch env.Life() {
	case state.Alive:
		err = m.envIsAlive(modelTag)
	case state.Dying:
		err = m.modelIsDying(modelTag)
	case state.Dead:
		err = m.envIsDead(modelTag)
	}

	return errors.Trace(err)
}

func (m *modelWorkerManager) envIsAlive(modelTag names.ModelTag) error {
	return m.runner.StartWorker(modelTag.Id(), func() (worker.Worker, error) {
		st, err := m.st.ForModel(modelTag)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to open state for model %s", modelTag.Id())
		}
		closeState := func() {
			err := st.Close()
			if err != nil {
				logger.Errorf("error closing state for model %s: %v", modelTag.Id(), err)
			}
		}

		envRunner, err := m.startModelWorker(m.st, st)
		if err != nil {
			closeState()
			return nil, errors.Trace(err)
		}

		// Close State when the runner for the model is done.
		go func() {
			envRunner.Wait()
			closeState()
		}()

		return envRunner, nil
	})
}

func dyingModelWorkerId(uuid string) string {
	return "dying" + ":" + uuid
}

// envNotFound stops all workers for that model.
func (m *modelWorkerManager) modelNotFound(modelTag names.ModelTag) error {
	uuid := modelTag.Id()
	if err := m.runner.StopWorker(uuid); err != nil {
		return errors.Trace(err)
	}
	if err := m.runner.StopWorker(dyingModelWorkerId(uuid)); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (m *modelWorkerManager) modelIsDying(modelTag names.ModelTag) error {
	id := dyingModelWorkerId(modelTag.Id())
	return m.runner.StartWorker(id, func() (worker.Worker, error) {
		st, err := m.st.ForModel(modelTag)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to open state for model %s", modelTag.Id())
		}
		closeState := func() {
			err := st.Close()
			if err != nil {
				logger.Errorf("error closing state for model %s: %v", modelTag.Id(), err)
			}
		}

		dyingRunner, err := m.dyingModelWorker(m.st, st)
		if err != nil {
			closeState()
			return nil, errors.Trace(err)
		}

		// Close State when the runner for the model is done.
		go func() {
			dyingRunner.Wait()
			closeState()
		}()

		return dyingRunner, nil
	})
}

func (m *modelWorkerManager) envIsDead(modelTag names.ModelTag) error {
	uuid := modelTag.Id()
	err := m.runner.StopWorker(uuid)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}
