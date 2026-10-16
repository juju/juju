// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dqliteconfigupdater

import (
	"context"
	"slices"
	"strings"

	"github.com/juju/worker/v5"

	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/errors"
	jworker "github.com/juju/juju/internal/worker"
)

// ControllerConfigService is the mutable source of truth for the Dqlite tuning
// values synchronised by this worker.
type ControllerConfigService interface {
	// GetDqliteConfig returns the mutable Dqlite tuning values of the controller
	// config.
	GetDqliteConfig(ctx context.Context) (controllerconfigservice.DqliteConfig, error)

	// WatchControllerConfig returns a watcher that emits an initial all-keys
	// event, and then the changed keys for any subsequent controller config
	// changes.
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

// Config contains the information required by the dqlite config updater.
type Config struct {
	// ControllerConfigService is the mutable source of truth for the three
	// Dqlite tuning values (query-tracing-enabled, query-tracing-threshold,
	// dqlite-busy-timeout) the worker reconciles into the local runtime config.
	ControllerConfigService ControllerConfigService

	// ReadRuntimeConfig reads this controller's local runtime.conf delivery
	// cache.
	ReadRuntimeConfig func() (controllerruntimeconfig.ControllerRuntimeConfig, error)

	// ChangeRuntimeConfig atomically mutates this controller's local
	// runtime.conf delivery cache.
	ChangeRuntimeConfig func(controllerruntimeconfig.Mutator) error

	// Logger is the logger used by the worker for its own messages.
	Logger corelogger.Logger
}

// Validate ensures that the necessary values are set in the structure.
func (c Config) Validate() error {
	if c.ControllerConfigService == nil {
		return errors.New("missing ControllerConfigService not valid").Add(coreerrors.NotValid)
	}
	if c.ReadRuntimeConfig == nil {
		return errors.New("missing ReadRuntimeConfig not valid").Add(coreerrors.NotValid)
	}
	if c.ChangeRuntimeConfig == nil {
		return errors.New("missing ChangeRuntimeConfig not valid").Add(coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.New("missing Logger not valid").Add(coreerrors.NotValid)
	}
	return nil
}

// NewWorker returns a worker that keeps this controller's local runtime.conf
// delivery cache in sync with the mutable Dqlite tuning values stored in the
// controller config domain service.
//
// The controller config service is the authoritative mutable source;
// runtime.conf is only the per-controller delivery cache that dbaccessor
// reads when it constructs the Dqlite application. On every watcher event
// carrying one of the three Dqlite keys the worker compares the Dqlite values
// with the local cache, and on a difference it writes all changed values
// together with one atomic ChangeControllerRuntimeConfig write, and only after
// that write succeeds does it return an error wrapping worker.ErrRestartAgent
// so the normal supervision path restarts the controller and dbaccessor
// constructs Dqlite with the new values. The initial all-keys watcher event is
// a reconciliation like any other: matching values establish the baseline
// without a write or a restart, so a restart after a write does not loop.
//
// A controller config, runtime config read, validation, or write failure
// is returned without wrapping ErrRestartAgent: no restart is requested,
// and the dependency engine applies its standard retry policy.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	return watcher.NewStringsWorker(watcher.StringsConfig{
		Handler: &dqliteConfigUpdater{config: config},
	})
}

// dqliteConfigUpdater implements watcher.StringsHandler.
type dqliteConfigUpdater struct {
	config Config
}

// SetUp implements watcher.StringsHandler. It establishes the watcher
// subscription before any state is read: the watcher's initial all-keys event
// is the readiness barrier, and the first Handle call performs the initial
// reconciliation.
func (w *dqliteConfigUpdater) SetUp(ctx context.Context) (watcher.StringsWatcher, error) {
	w.config.Logger.Infof(ctx, "dqlite config updater started")
	return w.config.ControllerConfigService.WatchControllerConfig(ctx)
}

// Handle implements watcher.StringsHandler. It reconciles the current Dqlite
// tuning values of the controller config with this controller's local
// runtime.conf delivery cache.
//
// Dying is handled by the strings worker, not here: its loop selects on the
// catacomb dying channel between events and passes the catacomb context into
// Handle, so a kill during this synchronous reconciliation aborts the
// controller config read through that context (an already completed runtime
// config write is safe to keep), and the loop then returns without delivering
// another event.
func (w *dqliteConfigUpdater) Handle(ctx context.Context, changes []string) error {
	if !slices.ContainsFunc(changes, isDqliteConfigKey) {
		return nil
	}
	// The mutable Dqlite values are re-read on every relevant event and compared
	// with the local delivery cache, so a value-identical event converges to no
	// difference.
	dqliteConfig, err := w.config.ControllerConfigService.GetDqliteConfig(ctx)
	if err != nil {
		return errors.Errorf("getting Dqlite controller config: %w", err)
	}
	rtConfig, err := w.config.ReadRuntimeConfig()
	if err != nil {
		return errors.Errorf("reading controller runtime config: %w", err)
	}

	var changed []string
	if dqliteConfig.QueryTracingEnabled != rtConfig.QueryTracingEnabled {
		changed = append(changed, controller.QueryTracingEnabled)
	}
	if dqliteConfig.QueryTracingThreshold != rtConfig.QueryTracingThreshold {
		changed = append(changed, controller.QueryTracingThreshold)
	}
	if dqliteConfig.DqliteBusyTimeout != rtConfig.DqliteBusyTimeout {
		changed = append(changed, controller.DqliteBusyTimeout)
	}
	if len(changed) == 0 {
		// The local delivery cache already carries the desired values: no write
		// and no restart. This is also the no-restart-loop outcome after the
		// controller restarts following a write.
		return nil
	}

	// Write all changed values together; the restart is requested below, only
	// after this atomic write succeeds.
	if err := w.config.ChangeRuntimeConfig(func(rtCfg *controllerruntimeconfig.ControllerRuntimeConfig) error {
		rtCfg.QueryTracingEnabled = dqliteConfig.QueryTracingEnabled
		rtCfg.QueryTracingThreshold = dqliteConfig.QueryTracingThreshold
		rtCfg.DqliteBusyTimeout = dqliteConfig.DqliteBusyTimeout
		return nil
	}); err != nil {
		return errors.Errorf("writing dqlite controller config to controller runtime config: %w", err)
	}

	w.config.Logger.Infof(ctx,
		"requesting controller restart for controller config change: %s",
		strings.Join(changed, ", "),
	)
	return errors.Errorf("%w: controller config changed: %s",
		jworker.ErrRestartAgent, strings.Join(changed, ", "))
}

// TearDown implements watcher.StringsHandler.
func (w *dqliteConfigUpdater) TearDown() error {
	w.config.Logger.Infof(context.Background(), "dqlite config updater stopped")
	return nil
}

// isDqliteConfigKey reports whether key is one of the mutable Dqlite tuning
// keys synchronised by this worker.
func isDqliteConfigKey(key string) bool {
	switch key {
	case controller.QueryTracingEnabled,
		controller.QueryTracingThreshold,
		controller.DqliteBusyTimeout:
		return true
	default:
		return false
	}
}
