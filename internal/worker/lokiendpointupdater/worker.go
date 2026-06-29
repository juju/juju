// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lokiendpointupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/logger"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// LoggerAPI represents the API calls the worker makes.
type LoggerAPI interface {
	// GetControllerLokiConfig returns the controller-wide Loki configuration
	// for the supplied agent.
	GetControllerLokiConfig(ctx context.Context, agentTag names.Tag) (logger.ControllerLokiConfig, error)

	// WatchControllerLokiConfig returns a watcher for controller-wide Loki
	// configuration changes.
	WatchControllerLokiConfig(ctx context.Context, agentTag names.Tag) (watcher.NotifyWatcher, error)
}

// WorkerConfig contains the information required by the worker.
type WorkerConfig struct {
	Agent              agent.Agent
	API                LoggerAPI
	AgentConfigChanged *voyeur.Value
	Logger             corelogger.Logger
}

// Validate ensures all the necessary fields have values.
func (c WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("missing agent")
	}
	if c.API == nil {
		return errors.NotValidf("missing api")
	}
	if c.AgentConfigChanged == nil {
		return errors.NotValidf("nil AgentConfigChanged")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

type lokiEndpointUpdater struct {
	config WorkerConfig
	tag    names.Tag
}

// NewWorker returns a worker that keeps the local agent config in sync with the
// controller-wide Loki endpoint configuration.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	currentConfig := config.Agent.CurrentConfig()
	w := &lokiEndpointUpdater{
		config: config,
		tag:    currentConfig.Tag(),
	}
	return watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: w,
	})
}

// SetUp implements watcher.NotifyHandler.
func (w *lokiEndpointUpdater) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	w.config.Logger.Infof(ctx, "loki endpoint updater worker started")
	if err := w.update(ctx); err != nil {
		return nil, errors.Trace(err)
	}
	return w.config.API.WatchControllerLokiConfig(ctx, w.tag)
}

// Handle implements watcher.NotifyHandler.
func (w *lokiEndpointUpdater) Handle(ctx context.Context) error {
	return errors.Trace(w.update(ctx))
}

// TearDown implements watcher.NotifyHandler.
func (w *lokiEndpointUpdater) TearDown() error {
	w.config.Logger.Infof(context.Background(), "loki endpoint updater worker stopped")
	return nil
}

func (w *lokiEndpointUpdater) update(ctx context.Context) error {
	lokiConfig, err := w.config.API.GetControllerLokiConfig(ctx, w.tag)
	if params.IsCodeNotFound(err) {
		lokiConfig = logger.ControllerLokiConfig{}
		err = nil
	}
	if err != nil {
		return errors.Annotate(err, "getting controller loki config")
	}

	currentConfig := w.config.Agent.CurrentConfig()
	if currentConfig.LokiEndpoint() == lokiConfig.Endpoint &&
		currentConfig.LokiCACert() == lokiConfig.CACert &&
		configInsecureEquals(currentConfig.LokiInsecureSkipVerify(), lokiConfig.InsecureSkipVerify) {
		return nil
	}

	var caCert *string
	if lokiConfig.CACert != "" {
		caCert = &lokiConfig.CACert
	}
	err = w.config.Agent.ChangeConfig(func(setter agent.ConfigSetter) error {
		setter.SetLokiConfig(lokiConfig.Endpoint, caCert, lokiConfig.InsecureSkipVerify)
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "updating agent loki config")
	}
	w.config.AgentConfigChanged.Set(true)
	return nil
}

func configInsecureEquals(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
