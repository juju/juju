// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
	stateChanged = "changed"
)

// ControllerConfigService is an interface for getting the controller config.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(ctx context.Context) (controller.Config, error)

	// WatchControllerConfig returns a watcher that returns keys for any changes
	// to controller config.
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

// AuditLogFactory is a function that will return an audit log given
// config.
type AuditLogFactory func(auditlog.Config) auditlog.AuditLog

type updater struct {
	internalStates          chan string
	catacomb                catacomb.Catacomb
	controllerConfigService ControllerConfigService

	mu         sync.Mutex
	current    auditlog.Config
	logFactory AuditLogFactory
}

// NewWorker returns a worker that will keep an up-to-date audit log config.
func NewWorker(controllerConfigService ControllerConfigService, initial auditlog.Config, logFactory AuditLogFactory) (worker.Worker, error) {
	return newWorker(controllerConfigService, initial, logFactory, nil)
}

func newWorker(
	controllerConfigService ControllerConfigService,
	initial auditlog.Config,
	logFactory AuditLogFactory,
	internalStates chan string,
) (*updater, error) {
	u := &updater{
		internalStates:          internalStates,
		controllerConfigService: controllerConfigService,
		current:                 initial,
		logFactory:              logFactory,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "audit-config-updater",
		Site: &u.catacomb,
		Work: u.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

// Kill is part of the worker.Worker interface.
func (u *updater) Kill() {
	u.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (u *updater) Wait() error {
	return u.catacomb.Wait()
}

func (u *updater) loop() error {
	ctx, cancel := u.scopedContext()
	defer cancel()

	changes, err := u.watchForConfigChanges(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// Report the initial started state.
	u.reportInternalState(stateStarted)

	for {
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()

		case _, ok := <-changes:
			if !ok {
				return errors.Errorf("watcher channel closed")
			}
			newConfig, err := u.newConfig(ctx)
			if err != nil {
				return errors.Annotatef(err, "getting new config")
			}
			u.update(newConfig)
		}
	}
}

func (u *updater) newConfig(ctx context.Context) (auditlog.Config, error) {
	cfg, err := u.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return auditlog.Config{}, errors.Trace(err)
	}
	result := auditlog.Config{
		Enabled:        cfg.AuditingEnabled(),
		CaptureAPIArgs: cfg.AuditLogCaptureArgs(),
		MaxSizeMB:      cfg.AuditLogMaxSizeMB(),
		MaxBackups:     cfg.AuditLogMaxBackups(),
		ExcludeMethods: cfg.AuditLogExcludeMethods(),
	}

	if result.Enabled && u.current.Target == nil {
		result.Target = u.logFactory(result)
	} else {
		// Keep the existing target to avoid file handle leaks from
		// disabling and enabling auditing - we'll still stop logging
		// because enabled is false.
		result.Target = u.current.Target
	}
	return result, nil
}

func (u *updater) update(newConfig auditlog.Config) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.current = newConfig

	// Report the initial started state.
	u.reportInternalState(stateChanged)
}

// CurrentConfig returns the updater's up-to-date audit config.
func (u *updater) CurrentConfig() auditlog.Config {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.current
}

// watchForConfigChanges starts a watcher for changes to controller config.
// It returns a channel which will receive events if the watcher fires.
func (u *updater) watchForConfigChanges(ctx context.Context) (<-chan []string, error) {
	watcher, err := u.controllerConfigService.WatchControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := u.catacomb.Add(watcher); err != nil {
		return nil, errors.Trace(err)
	}

	// Consume the initial events from the watchers. The watcher will
	// dispatch an initial event when it is created, so we need to consume
	// that event before we can start watching.
	if _, err := eventsource.ConsumeInitialEvent[[]string](ctx, watcher); err != nil {
		return nil, errors.Trace(err)
	}

	return watcher.Changes(), nil
}

func (u *updater) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(u.catacomb.Context(context.Background()))
}

func (u *updater) reportInternalState(state string) {
	select {
	case <-u.catacomb.Dying():
	case u.internalStates <- state:
	default:
	}
}
