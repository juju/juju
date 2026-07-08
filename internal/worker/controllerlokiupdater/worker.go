// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlokiupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/logging"
	loggingerrors "github.com/juju/juju/domain/logging/errors"
)

// LokiConfigService provides access to controller Loki configuration.
type LokiConfigService interface {
	// GetLokiConfig returns the current controller-wide Loki configuration.
	// If no config exists, an error satisfying loggingerrors.LokiConfigNotFound
	// is returned.
	GetLokiConfig(ctx context.Context) (logging.LokiConfig, error)

	// WatchLokiConfig returns a watcher that fires when the controller-wide
	// Loki configuration changes.
	WatchLokiConfig(ctx context.Context) (watcher.NotifyWatcher, error)
}

// Config contains the information required by the controller loki config
// updater worker.
type Config struct {
	// LokiConfigService provides access to controller Loki configuration.
	LokiConfigService LokiConfigService

	// WriteLokiConfig persists the supplied Loki config to the controller
	// runtime config file. The worker calls this on every config change
	// so the logrouter can re-read current values from runtime.conf.
	WriteLokiConfig func(logging.LokiConfig) error

	// NotifyConfigReload signals the controller config change socket so
	// that downstream workers (including the logrouter) re-read their
	// persisted config.
	NotifyConfigReload func() error

	// Logger is the logger used by the worker for its own messages.
	Logger corelogger.Logger
}

// Validate ensures all the necessary fields have values.
func (c Config) Validate() error {
	if c.LokiConfigService == nil {
		return errors.NotValidf("nil LokiConfigService")
	}
	if c.WriteLokiConfig == nil {
		return errors.NotValidf("nil WriteLokiConfig")
	}
	if c.NotifyConfigReload == nil {
		return errors.NotValidf("nil NotifyConfigReload")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewWorker returns a worker that keeps the controller runtime config in
// sync with the controller-wide Loki configuration stored in the logging
// domain. When the Loki config changes, the worker writes the new values
// to runtime.conf and signals the config-change socket so the logrouter
// picks up the update.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &lokiConfigUpdater{config: config}
	worker, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: w,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

type lokiConfigUpdater struct {
	config Config
}

// SetUp implements watcher.NotifyHandler. It performs an initial sync of the
// Loki config to runtime.conf and returns the watcher for future changes.
func (w *lokiConfigUpdater) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	w.config.Logger.Infof(ctx, "controller loki config updater started")

	if err := w.syncConfig(ctx); err != nil {
		return nil, errors.Trace(err)
	}
	return w.config.LokiConfigService.WatchLokiConfig(ctx)
}

// Handle implements watcher.NotifyHandler. It re-reads the current Loki
// config from the domain service and writes it to runtime.conf.
func (w *lokiConfigUpdater) Handle(ctx context.Context) error {
	return errors.Trace(w.syncConfig(ctx))
}

// TearDown implements watcher.NotifyHandler.
func (w *lokiConfigUpdater) TearDown() error {
	w.config.Logger.Infof(context.Background(), "controller loki config updater stopped")
	return nil
}

// syncConfig reads the current Loki config from the domain service, writes
// it to the runtime config file, and signals the config-change socket. If
// no Loki config is found (i.e. Loki has been removed), empty values are
// written so the logrouter defaults to logsink mode.
func (w *lokiConfigUpdater) syncConfig(ctx context.Context) error {
	lokiConfig, err := w.config.LokiConfigService.GetLokiConfig(ctx)
	if errors.Is(err, loggingerrors.LokiConfigNotFound) {
		lokiConfig = logging.LokiConfig{}
		err = nil
	}
	if err != nil {
		return errors.Annotate(err, "getting controller loki config")
	}

	if err := w.config.WriteLokiConfig(lokiConfig); err != nil {
		return errors.Annotate(err, "writing loki config to runtime config")
	}

	w.config.Logger.Infof(ctx,
		"controller loki config updated: endpoint=%q",
		lokiConfig.Endpoint,
	)

	if err := w.config.NotifyConfigReload(); err != nil {
		return errors.Annotate(err, "notifying config reload")
	}
	return nil
}
