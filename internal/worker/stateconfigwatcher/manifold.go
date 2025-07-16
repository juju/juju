// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconfigwatcher

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	coreagent "github.com/juju/juju/core/agent"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.worker.stateconfigwatcher")

type ManifoldConfig struct {
	AgentName          string
	AgentConfigChanged *voyeur.Value
}

// Manifold returns a dependency.Manifold which wraps the machine
// agent's voyeur.Value which gets set whenever the machine agent's
// config is changed. Whenever the config is updated the presence of
// state serving info is checked and if state serving info was added
// or removed the manifold worker will bounce itself.
//
// The manifold offers a single boolean output which will be true if
// state serving info is available (i.e. the machine agent should be a
// state server) and false otherwise.
//
// This manifold is intended to be used as a dependency for the state
// manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			if config.AgentConfigChanged == nil {
				return nil, errors.NotValidf("nil AgentConfigChanged")
			}

			tagKind := a.CurrentConfig().Tag().Kind()
			if !coreagent.IsAllowedControllerTag(tagKind) {
				return nil, errors.New("manifold can only be used with a machine or controller agent")
			}

			w := &stateConfigWatcher{
				agent:              a,
				agentConfigChanged: config.AgentConfigChanged,
			}
			w.tomb.Go(w.loop)
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
		*outPointer = inWorker.isControllerAgent()
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

func (w *stateConfigWatcher) isControllerAgent() bool {
	config := w.agent.CurrentConfig()
	_, ok := config.ControllerAgentInfo()
	return ok
}

func (w *stateConfigWatcher) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	watch := w.agentConfigChanged.Watch()
	defer watch.Close()

	lastValue := w.isControllerAgent()

	watchCh := make(chan bool)
	go func() {
		for {
			if watch.Next() {
				select {
				case <-w.tomb.Dying():
					return
				case watchCh <- true:
				}
			} else {
				// watcher or voyeur.Value closed.
				close(watchCh)
				return
			}
		}
	}()

	for {
		select {
		case <-w.tomb.Dying():
			logger.Infof(ctx, "tomb dying")
			return tomb.ErrDying
		case _, ok := <-watchCh:
			if !ok {
				return errors.New("config changed value closed")
			}
			if w.isControllerAgent() != lastValue {
				// Controller agent info has been set or unset so restart so
				// that dependents get notified. ErrBounce ensures that the
				// manifold is restarted quickly.
				logger.Debugf(ctx, "controller agent info change in agent config")
				return dependency.ErrBounce
			}
		}
	}
}

// Kill implements worker.Worker.
func (w *stateConfigWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *stateConfigWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *stateConfigWatcher) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
