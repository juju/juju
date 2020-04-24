// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/state"
)

// ConfigSource lets us get notifications of changes to controller
// configuration, and then get the changed config. (Primary
// implementation is State.)
type ConfigSource interface {
	WatchControllerConfig() state.NotifyWatcher
	ControllerConfig() (controller.Config, error)
}

// AuditLogFactory is a function that will return an audit log given
// config.
type AuditLogFactory func(auditlog.Config) auditlog.AuditLog

// New returns a worker that will keep an up-to-date audit log config.
func New(source ConfigSource, initial auditlog.Config, logFactory AuditLogFactory) (worker.Worker, error) {
	u := &updater{
		source:     source,
		current:    initial,
		logFactory: logFactory,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &u.catacomb,
		Work: u.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

type updater struct {
	mu         sync.Mutex
	catacomb   catacomb.Catacomb
	source     ConfigSource
	current    auditlog.Config
	logFactory AuditLogFactory
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
	watcher := u.source.WatchControllerConfig()
	if err := u.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		case _, ok := <-watcher.Changes():
			if !ok {
				return errors.Errorf("watcher channel closed")
			}
			newConfig, err := u.newConfig()
			if err != nil {
				return errors.Annotatef(err, "getting new config")
			}
			u.update(newConfig)
		}
	}
}

func (u *updater) newConfig() (auditlog.Config, error) {
	cfg, err := u.source.ControllerConfig()
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
}

// CurrentConfig returns the updater's up-to-date audit config.
func (u *updater) CurrentConfig() auditlog.Config {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.current
}
