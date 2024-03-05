// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"context"

	"github.com/juju/worker/v4"

	"github.com/juju/juju/controller"
)

// NewWorkerShim calls through to NewWorker, and exists only
// to adapt to the signature of ManifoldConfig.NewWorker.
func NewWorkerShim(config Config) (worker.Worker, error) {
	return NewWorker(config)
}

// ControllerConfigGetter is an interface that returns the controller config.
type ControllerConfigGetter interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// GetControllerConfig gets the controller config from a *State - it
// exists so we can test the manifold without a StateSuite.
func GetControllerConfig(ctx context.Context, getter ControllerConfigGetter) (controller.Config, error) {
	return getter.ControllerConfig(ctx)
}
