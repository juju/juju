// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/socketlistener"
)

// SocketListener describes a worker that listens on a unix socket.
type SocketListener interface {
	worker.Worker
}

// NewSocketListener returns a new socket listener with the desired config.
func NewSocketListener(config socketlistener.Config) (SocketListener, error) {
	return socketlistener.NewSocketListener(config)
}

// ManifoldConfig defines the configuration for the agent controller config
// manifold.
type ManifoldConfig struct {
	AgentName string
	Logger    logger.Logger
	Clock     clock.Clock
	// SocketName is the socket file descriptor.
	SocketName string
	// NewSocketListener is the function that creates a new socket listener.
	NewSocketListener func(socketlistener.Config) (SocketListener, error)
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if cfg.NewSocketListener == nil {
		return errors.NotValidf("nil NewSocketListener func")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
		},
		Output: configOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var thisAgent agent.Agent
			if err := getter.Get(config.AgentName, &thisAgent); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				ControllerID:      thisAgent.CurrentConfig().Tag().Id(),
				Logger:            config.Logger,
				Clock:             config.Clock,
				NewSocketListener: config.NewSocketListener,
				SocketName:        config.SocketName,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
	}
}

func configOutput(in worker.Worker, out any) error {
	w, ok := in.(*configWorker)
	if !ok {
		return errors.Errorf("expected configWorker, got %T", in)
	}
	switch out := out.(type) {
	case *ConfigWatcher:
		target, err := w.Watcher()
		if err != nil {
			return errors.Trace(err)
		}
		*out = target
	default:
		return errors.Errorf("unsupported output of *ConfigWatcher type, got %T", out)
	}
	return nil
}
