// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher/legacy"
)

type updater struct {
	source     ConfigSource
	current    auditlog.Config
	logFactory AuditLogFactory
	changes    chan<- auditlog.Config
}

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

// New returns a worker that will publish changed audit log config to
// the channel passed in.
func New(source ConfigSource, initial auditlog.Config, logFactory AuditLogFactory, changes chan<- auditlog.Config) (worker.Worker, error) {
	// This uses legacy.NewNotifyWorker because it needs to run
	// against State - it feeds audit config changes to the API
	// server, so it can't make requests via the API
	// server. (legacy.NewNotifyWorker exists because the
	// state.NotifyWorker and watcher.NotifyWorker interfaces are
	// incompatible, Changes returns different types.)
	return legacy.NewNotifyWorker(&updater{
		source:     source,
		current:    initial,
		logFactory: logFactory,
		changes:    changes,
	}), nil
}

// Setup implements watcher.NotifyHandler.
func (u *updater) SetUp() (state.NotifyWatcher, error) {
	return u.source.WatchControllerConfig(), nil
}

// Handle implements watcher.NotifyHandler.
func (u *updater) Handle(abort <-chan struct{}) error {
	cConfig, err := u.source.ControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	newConfig := u.auditConfigFrom(cConfig)
	if changed(u.current, newConfig) {
		u.current = newConfig
		// Allow the send to be cancelled.
		select {
		case <-abort:
		case u.changes <- newConfig:
		}
	}
	return nil
}

// TearDown implements watcher.NotifyHandler.
func (u *updater) TearDown() error {
	return nil
}

func (u *updater) auditConfigFrom(cfg controller.Config) auditlog.Config {
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
	return result
}

func changed(old, new auditlog.Config) bool {
	return old.Enabled != new.Enabled ||
		old.CaptureAPIArgs != new.CaptureAPIArgs ||
		setsDiffer(old.ExcludeMethods, new.ExcludeMethods)
}

func setsDiffer(a, b set.Strings) bool {
	return !a.Difference(b).IsEmpty() || !b.Difference(a).IsEmpty()
}
