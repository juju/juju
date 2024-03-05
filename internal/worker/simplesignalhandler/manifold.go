// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplesignalhandler

import (
	"context"
	"fmt"
	"os"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
)

type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
}

// ManifoldConfig is responsible for configuring this worker.
type ManifoldConfig struct {
	// SignalCh is a preconfigured channel listening for signals.
	SignalCh <-chan os.Signal

	// DefaultHandlerError is the default error to return from this worker when
	// there is no mapping for the received signal. Value must be specified.
	DefaultHandlerError error

	// HandlerErrors is a map of os.Signal to error returns from this worker.
	// Valid for this map to be nil or empty.
	HandlerErrors map[os.Signal]error

	// Logger to use for this worker
	Logger Logger
}

// Manifold returns the dependency manifold for this worker based on the config
// provided.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: nil,
		Output: nil,
		Start:  config.Start,
	}
}

// Start is responsible for creating a new worker for the manifold config.
func (m ManifoldConfig) Start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}

	return NewSignalWatcher(
		m.Logger,
		m.SignalCh,
		SignalHandler(m.DefaultHandlerError, m.HandlerErrors),
	)
}

// Validate validates the manifold config.
func (m ManifoldConfig) Validate() error {
	if m.SignalCh == nil {
		return fmt.Errorf("%w SignalCh cannot be nil", errors.NotValid)
	}

	if m.DefaultHandlerError == nil {
		return fmt.Errorf("%w DefaultHandlerError cannot be nil", errors.NotValid)
	}
	return nil
}
