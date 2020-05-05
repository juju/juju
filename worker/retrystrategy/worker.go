// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

// Facade defines the capabilities required by the worker from the API.
type Facade interface {
	RetryStrategy(names.Tag) (params.RetryStrategy, error)
	WatchRetryStrategy(names.Tag) (watcher.NotifyWatcher, error)
}

// WorkerConfig defines the worker's dependencies.
type WorkerConfig struct {
	Facade        Facade
	AgentTag      names.Tag
	RetryStrategy params.RetryStrategy
	Logger        Logger
}

// Validate returns an error if the configuration is not complete.
func (c WorkerConfig) Validate() error {
	if c.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if c.AgentTag == nil {
		return errors.NotValidf("nil AgentTag")
	}
	empty := params.RetryStrategy{}
	if c.RetryStrategy == empty {
		return errors.NotValidf("empty RetryStrategy")
	}
	return nil
}

// RetryStrategyWorker is a NotifyWorker with one additional
// method that returns the current retry strategy.
type RetryStrategyWorker struct {
	*watcher.NotifyWorker
	retryStrategy params.RetryStrategy
}

// NewRetryStrategyWorker returns a worker.Worker that returns the current
// retry strategy and bounces when it changes.
func NewRetryStrategyWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: retryStrategyHandler{config},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &RetryStrategyWorker{w, config.RetryStrategy}, nil
}

// GetRetryStrategy returns the current hook retry strategy
func (w *RetryStrategyWorker) GetRetryStrategy() params.RetryStrategy {
	return w.retryStrategy
}

// retryStrategyHandler implements watcher.NotifyHandler
type retryStrategyHandler struct {
	config WorkerConfig
}

// SetUp is part of the watcher.NotifyHandler interface.
func (h retryStrategyHandler) SetUp() (watcher.NotifyWatcher, error) {
	return h.config.Facade.WatchRetryStrategy(h.config.AgentTag)
}

// Handle is part of the watcher.NotifyHandler interface.
// Whenever a valid change is encountered the worker bounces,
// making the dependents bounce and get the new value
func (h retryStrategyHandler) Handle(_ <-chan struct{}) error {
	newRetryStrategy, err := h.config.Facade.RetryStrategy(h.config.AgentTag)
	if err != nil {
		return errors.Trace(err)
	}
	if newRetryStrategy != h.config.RetryStrategy {
		h.config.Logger.Debugf("bouncing retrystrategy worker to get new values")
		return dependency.ErrBounce
	}
	return nil
}

// TearDown is part of the watcher.NotifyHandler interface.
func (h retryStrategyHandler) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	return nil
}
