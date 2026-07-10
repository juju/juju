// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"context"
	"os"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/socketlistener"
	"github.com/juju/juju/internal/worker/gate"
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
	// ControllerID is the numeric ID of this controller.
	ControllerID string
	Logger       logger.Logger
	// SocketName is the socket file descriptor.
	SocketName string
	// SocketFileMode is the file mode to apply to the created Unix socket.
	// A zero value means the default 0700 (owner-only) is used.
	SocketFileMode os.FileMode
	// NewSocketListener is the function that creates a new socket listener.
	NewSocketListener func(socketlistener.Config) (SocketListener, error)
	// ReadyUnlocker is unlocked once the socket listener is successfully
	// started, signalling to dependents (e.g. the deployer) that the socket
	// is available.
	ReadyUnlocker gate.Unlocker
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.ControllerID == "" {
		return errors.NotValidf("empty ControllerID")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if cfg.NewSocketListener == nil {
		return errors.NotValidf("nil NewSocketListener func")
	}
	if cfg.ReadyUnlocker == nil {
		return errors.NotValidf("nil ReadyUnlocker")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: configOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				ControllerID:      config.ControllerID,
				Logger:            config.Logger,
				Clock:             clock.WallClock,
				NewSocketListener: config.NewSocketListener,
				SocketName:        config.SocketName,
				SocketFileMode:    config.SocketFileMode,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			// Socket listener started successfully; the socket file now exists.
			// Unblock any manifolds waiting for it (e.g. the deployer).
			config.ReadyUnlocker.Unlock()

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
