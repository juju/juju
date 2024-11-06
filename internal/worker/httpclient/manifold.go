// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	internalhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/worker/common"
)

// HTTPClientWorker is the interface for the http client worker.
type HTTPClientWorker interface {
	corehttp.HTTPClient
	worker.Worker
}

// NewHTTPClientFunc is the function signature for creating a new http client.
type NewHTTPClientFunc func(string, ...internalhttp.Option) *internalhttp.Client

// HTTPClientWorkerFunc is the function signature for creating a new
// http client worker.
type HTTPClientWorkerFunc func(*internalhttp.Client) (worker.Worker, error)

// ManifoldConfig defines the configuration for the http client manifold.
type ManifoldConfig struct {
	NewHTTPClient       NewHTTPClientFunc
	NewHTTPClientWorker HTTPClientWorkerFunc
	Clock               clock.Clock
	Logger              logger.Logger
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.NewHTTPClient == nil {
		return errors.NotValidf("nil NewHTTPClient")
	}
	if cfg.NewHTTPClientWorker == nil {
		return errors.NotValidf("nil NewHTTPClientWorker")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the http client worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: output,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				NewHTTPClient:       config.NewHTTPClient,
				NewHTTPClientWorker: config.NewHTTPClientWorker,
				Clock:               config.Clock,
				Logger:              config.Logger,
			})
			return w, errors.Trace(err)
		},
	}
}

func output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*httpClientWorker)
	if !ok {
		return errors.Errorf("expected input of httpClientWorker, got %T", in)
	}

	switch out := out.(type) {
	case *corehttp.HTTPClientGetter:
		var target corehttp.HTTPClientGetter = w
		*out = target
	default:
		return errors.Errorf("expected output of HTTPClientGetter, got %T", out)
	}
	return nil
}

// NewHTTPClient creates a new http client.
func NewHTTPClient(opts ...internalhttp.Option) *internalhttp.Client {
	return internalhttp.NewClient(opts...)
}
