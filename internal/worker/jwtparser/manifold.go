// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/internal/jwtparser"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	StateName string
}

// Manifold returns a manifold whose worker wraps a JWT parser.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{config.StateName}
	return dependency.Manifold{
		Inputs: inputs,
		Output: outputFunc,
		Start:  config.start,
	}
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}
	systemState, err := statePool.SystemState()
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	// The statePool is only needed for worker creation
	// currently but should be improved to watch for changes.
	w, err := NewWorker(systemState, DefaultHTTPClient())
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() {
		_ = stTracker.Done()
	}), nil
}

// outputFunc extracts a jwtparser.Parser from a
// jwtParserWorker contained within a CleanupWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	inWorker, _ := in.(*jwtParserWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case **jwtparser.Parser:
		*outPointer = inWorker.jwtParser
	default:
		return errors.Errorf("out should be jwtparser.Parser; got %T", out)
	}
	return nil
}

// DefaultHTTPClient returns a defaulthttp client
// that follows redirects with a sensible timeout.
func DefaultHTTPClient() HTTPClient {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
