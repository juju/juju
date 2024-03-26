// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	jujuagent "github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/common"
)

// GetControllerConfigServiceFunc is a helper function that gets a service from
// the manifold.
type GetControllerConfigServiceFunc func(getter dependency.Getter, name string) (ControllerConfigService, error)

// ManifoldConfig holds the information needed to run an
// auditconfigupdater in a dependency.Engine.
type ManifoldConfig struct {
	AgentName                  string
	ServiceFactoryName         string
	NewWorker                  func(ControllerConfigService, auditlog.Config, AuditLogFactory) (worker.Worker, error)
	GetControllerConfigService GetControllerConfigServiceFunc
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.GetControllerConfigService == nil {
		return errors.NotValidf("nil GetControllerConfigService")
	}
	return nil
}

// Manifold returns a dependency.Manifold to run an
// auditconfigupdater.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ServiceFactoryName,
		},
		Start:  config.start,
		Output: output,
	}
}

func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (_ worker.Worker, err error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent jujuagent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigService, err := config.GetControllerConfigService(getter, config.ServiceFactoryName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logDir := agent.CurrentConfig().LogDir()

	logFactory := func(cfg auditlog.Config) auditlog.AuditLog {
		return auditlog.NewLogFile(logDir, cfg.MaxSizeMB, cfg.MaxBackups)
	}
	auditConfig, err := initialConfig(controllerConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if auditConfig.Enabled {
		auditConfig.Target = logFactory(auditConfig)
	}

	w, err := config.NewWorker(controllerConfigService, auditConfig, logFactory)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
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

func initialConfig(cfg controller.Config) (auditlog.Config, error) {
	result := auditlog.Config{
		Enabled:        cfg.AuditingEnabled(),
		CaptureAPIArgs: cfg.AuditLogCaptureArgs(),
		MaxSizeMB:      cfg.AuditLogMaxSizeMB(),
		MaxBackups:     cfg.AuditLogMaxBackups(),
		ExcludeMethods: cfg.AuditLogExcludeMethods(),
	}
	return result, nil
}

// GetControllerConfigService is a helper function that gets a service from the
// manifold.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory servicefactory.ControllerServiceFactory) ControllerConfigService {
		return factory.ControllerConfig()
	})
}
