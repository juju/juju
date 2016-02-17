// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconfigwatcher

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/voyeur"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type ManifoldConfig struct {
	AgentName          string
	AgentConfigChanged *voyeur.Value
}

// Manifold returns a dependency.Manifold which wraps the machine
// agent's voyeur.Value which gets set whenever it the machine agnet's
// config is changed. Whenever the config is updated the presence of
// state serving info is checked and if state serving info was added
// or removed the manifold worker will bounce itself.
//
// The manifold offes a single boolean output which will be true if
// state serving info is available (i.e. the machine agent should be a
// state server) and false otherwise.
//
// This manifold is intended to be used as a dependency for the state
// manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var a agent.Agent
			if err := getResource(config.AgentName, &a); err != nil {
				return nil, err
			}

			if config.AgentConfigChanged == nil {
				return nil, errors.NotValidf("nil AgentConfigChanged")
			}

			if _, ok := a.CurrentConfig().Tag().(names.MachineTag); !ok {
				return nil, errors.New("manifold can only be used with a machine agent")
			}

			w := &stateConfigWatcher{
				agent:              a,
				agentConfigChanged: config.AgentConfigChanged,
			}
			go func() {
				defer w.tomb.Done()
				w.tomb.Kill(w.loop())
			}()
			return w, nil
		},
		Output: outputFunc,
	}
}

// outputFunc extracts a bool from a *stateConfigWatcher. If true, the
// agent is a state server.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*stateConfigWatcher)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}
	switch outPointer := out.(type) {
	case *bool:
		*outPointer = inWorker.isStateServer()
	default:
		return errors.Errorf("out should be *bool; got %T", out)
	}
	return nil
}

type stateConfigWatcher struct {
	tomb               tomb.Tomb
	agent              agent.Agent
	agentConfigChanged *voyeur.Value
}

func (w *stateConfigWatcher) isStateServer() bool {
	config := w.agent.CurrentConfig()
	_, ok := config.StateServingInfo()
	return ok
}

func (w *stateConfigWatcher) loop() error {
	watch := w.agentConfigChanged.Watch()
	defer watch.Close()

	lastValue := w.isStateServer()

	watchCh := make(chan bool)
	go func() {
		// Consume the initial event to avoid unnecessary worker
		// restart churn.
		if !watch.Next() {
			return
		}

		for {
			if watch.Next() {
				select {
				case watchCh <- true:
				case <-w.tomb.Dying():
					return
				}
			}
		}
	}()

	for {
		select {
		case <-watchCh:
			if w.isStateServer() != lastValue {
				// State serving info has been set or unset so restart
				// so that dependents get notified. ErrBounce ensures
				// that the manifold is restarted quickly.
				return dependency.ErrBounce
			}
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *stateConfigWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *stateConfigWatcher) Wait() error {
	return w.tomb.Wait()
}
