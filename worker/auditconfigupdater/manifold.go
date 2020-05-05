// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	jujuagent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information needed to run an
// auditconfigupdater in a dependency.Engine.
type ManifoldConfig struct {
	AgentName string
	StateName string
	NewWorker func(ConfigSource, auditlog.Config, AuditLogFactory) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold to run an
// auditconfigupdater.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.StateName,
		},
		Start:  config.start,
		Output: output,
	}
}

func (config ManifoldConfig) start(context dependency.Context) (_ worker.Worker, err error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent jujuagent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			stTracker.Done()
		}
	}()

	logDir := agent.CurrentConfig().LogDir()

	st := statePool.SystemState()

	logFactory := func(cfg auditlog.Config) auditlog.AuditLog {
		return auditlog.NewLogFile(logDir, cfg.MaxSizeMB, cfg.MaxBackups)
	}
	auditConfig, err := initialConfig(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if auditConfig.Enabled {
		auditConfig.Target = logFactory(auditConfig)
	}

	w, err := config.NewWorker(st, auditConfig, logFactory)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { stTracker.Done() }), nil
}

type withCurrentConfig interface {
	CurrentConfig() auditlog.Config
}

func output(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(withCurrentConfig)
	if !ok {
		return errors.Errorf("expected worker implementing CurrentConfig(), got %T", in)
	}
	target, ok := out.(*func() auditlog.Config)
	if !ok {
		return errors.Errorf("out should be *func() auditlog.Config; got %T", out)
	}
	*target = w.CurrentConfig
	return nil
}

func initialConfig(source ConfigSource) (auditlog.Config, error) {
	cfg, err := source.ControllerConfig()
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
	return result, nil
}
