// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	internalhttp "github.com/juju/juju/internal/http"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// WorkerConfig encapsulates the configuration options for the http client
// worker.
type WorkerConfig struct {
	NewHTTPClient       NewHTTPClientFunc
	NewHTTPClientWorker HTTPClientWorkerFunc
	Clock               clock.Clock
	Logger              logger.Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.NewHTTPClient == nil {
		return errors.NotValidf("nil NewHTTPClient")
	}
	if c.NewHTTPClientWorker == nil {
		return errors.NotValidf("nil NewHTTPClientWorker")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// httpClientRequest is used to pass requests for Storage Registry
// instances into the worker loop.
type httpClientRequest struct {
	purpose corehttp.Purpose
	done    chan error
}

type httpClientWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	catacomb       catacomb.Catacomb

	runner *worker.Runner

	httpClientRequests chan httpClientRequest
}

// NewWorker creates a new object store worker.
func NewWorker(cfg WorkerConfig) (*httpClientWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*httpClientWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:  "http-client",
		Clock: cfg.Clock,
		IsFatal: func(err error) bool {
			return false
		},
		RestartDelay: time.Second * 10,
		Logger:       internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &httpClientWorker{
		internalStates:     internalStates,
		cfg:                cfg,
		runner:             runner,
		httpClientRequests: make(chan httpClientRequest),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "http-client",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.runner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *httpClientWorker) loop() (err error) {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx := w.catacomb.Context(context.Background())

	for {
		select {
		// The following ensures that all httpClientRequests are serialised and
		// processed in order.
		case req := <-w.httpClientRequests:
			if err := w.initHTTPClient(ctx, req.purpose); err != nil {
				select {
				case req.done <- errors.Trace(err):
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				}
				continue
			}

			select {
			case req.done <- nil:
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			}

		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *httpClientWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *httpClientWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetHTTPClient returns a httpClient for the given purpose.
// TODO (stickupkid): Currently we don't pass the namespace (model uuid) to
// get a client for a specific model with the associated model-config.
func (w *httpClientWorker) GetHTTPClient(ctx context.Context, purpose corehttp.Purpose) (corehttp.HTTPClient, error) {
	// First check if we've already got the httpClient worker already running.
	// If we have, then return out quickly. The httpClientRunner is the cache,
	// so there is no need to have an in-memory cache here.
	if httpClient, err := w.workerFromCache(purpose); err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, corehttp.ErrHTTPClientDying
		}

		return nil, errors.Trace(err)
	} else if httpClient != nil {
		return httpClient, nil
	}

	// Enqueue the request as it's either starting up and we need to wait longer
	// or it's not running and we need to start it.
	req := httpClientRequest{
		purpose: purpose,
		done:    make(chan error),
	}
	select {
	case w.httpClientRequests <- req:
	case <-w.catacomb.Dying():
		return nil, corehttp.ErrHTTPClientDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case err := <-req.done:
		// If we know we've got an error, just return that error before
		// attempting to ask the objectStoreRunnerWorker.
		if err != nil {
			return nil, errors.Trace(err)
		}
	case <-w.catacomb.Dying():
		return nil, corehttp.ErrHTTPClientDying
	case <-ctx.Done():
		return nil, errors.Trace(ctx.Err())
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.runner.Worker(purpose.String(), w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if tracked == nil {
		return nil, errors.NotFoundf("httpclient")
	}
	return tracked.(corehttp.HTTPClient), nil
}

func (w *httpClientWorker) workerFromCache(purpose corehttp.Purpose) (corehttp.HTTPClient, error) {
	// If the worker already exists, return the existing worker early.
	if httpClient, err := w.runner.Worker(purpose.String(), w.catacomb.Dying()); err == nil {
		return httpClient.(corehttp.HTTPClient), nil
	} else if errors.Is(errors.Cause(err), worker.ErrDead) {
		// Handle the case where the runner is dead due to this worker dying.
		select {
		case <-w.catacomb.Dying():
			return nil, w.catacomb.ErrDying()
		default:
			return nil, errors.Trace(err)
		}
	} else if !errors.Is(errors.Cause(err), errors.NotFound) {
		// If it's not a NotFound error, return the underlying error. We should
		// only start a worker if it doesn't exist yet.
		return nil, errors.Trace(err)
	}
	// We didn't find the worker, so return nil, we'll create it in the next
	// step.
	return nil, nil
}

func (w *httpClientWorker) initHTTPClient(ctx context.Context, purpose corehttp.Purpose) error {
	err := w.runner.StartWorker(ctx, purpose.String(), func(ctx context.Context) (worker.Worker, error) {
		// TODO (stickupkid): We can pass in additional configuration here if
		// needed.
		httpClient := w.cfg.NewHTTPClient(purpose, internalhttp.WithLogger(w.cfg.Logger))

		worker, err := w.cfg.NewHTTPClientWorker(httpClient)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return worker, nil
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

func (w *httpClientWorker) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
