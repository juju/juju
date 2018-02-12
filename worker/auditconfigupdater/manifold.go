// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	jujuagent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/dependency"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information needed to run an
// auditconfigupdater in a dependency.Engine.
type ManifoldConfig struct {
	AgentName string
	StateName string
	NewWorker func(ConfigSource, auditlog.Config, AuditLogFactory, chan<- auditlog.Config) (worker.Worker, error)
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
	md := &manifoldData{
		cfg:     config,
		changes: make(chan auditlog.Config, 1),
	}
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.StateName,
		},
		Start:  md.start,
		Output: md.getOutput,
	}
}

// Output defines what values the auditconfigupdater provides to workers that
// depend on it.
type Output interface {
	Config() auditlog.Config
	Changes() <-chan auditlog.Config
}

// manifoldData holds values needed for running the manifold and
// providing outputs. It would have been called manifoldState if state
// wasn't severely overused.
type manifoldData struct {
	cfg         ManifoldConfig
	auditConfig auditlog.Config
	changes     chan auditlog.Config
}

func (md *manifoldData) start(context dependency.Context) (_ worker.Worker, err error) {
	if err := md.cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent jujuagent.Agent
	if err := context.Get(md.cfg.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(md.cfg.StateName, &stTracker); err != nil {
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
	md.auditConfig, err = initialConfig(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if md.auditConfig.Enabled {
		md.auditConfig.Target = logFactory(md.auditConfig)
	}

	w, err := md.cfg.NewWorker(st, md.auditConfig, logFactory, md.changes)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { stTracker.Done() }), nil
}

func (md *manifoldData) getOutput(_ worker.Worker, out interface{}) error {
	// We don't need anything from the worker, all the outputs are
	// generated at start time.
	switch target := out.(type) {
	case *Output:
		*target = md
	default:
		return errors.Errorf("out should be *auditconfigupdater.Output; got %T", out)
	}
	return nil
}

// Config implements Output.
func (md *manifoldData) Config() auditlog.Config {
	return md.auditConfig
}

// Changes implements Output.
func (md *manifoldData) Changes() <-chan auditlog.Config {
	return md.changes
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
