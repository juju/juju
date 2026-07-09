// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlokiupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/logging"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig defines the configuration for the controller loki config
// updater manifold.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain-services dependency.
	DomainServicesName string

	// RuntimeConfigPath is the path to the controller runtime config file.
	RuntimeConfigPath string

	// ConfigChangeSocketPath is the path to the controller config change
	// unix socket. The worker signals this socket after persisting a Loki
	// config change so downstream workers re-read their config.
	ConfigChangeSocketPath string

	// Logger is the logger used by the worker.
	Logger corelogger.Logger
}

// Validate checks that all required configuration fields are set.
func (c ManifoldConfig) Validate() error {
	if c.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if c.RuntimeConfigPath == "" {
		return errors.NotValidf("empty RuntimeConfigPath")
	}
	if c.ConfigChangeSocketPath == "" {
		return errors.NotValidf("empty ConfigChangeSocketPath")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that watches for controller Loki
// configuration changes in the logging domain and persists them to the
// controller runtime config file. After each write it signals the
// config-change socket so the logrouter worker picks up the update.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

func (c ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	domainServices, err := getControllerDomainServices(getter, c.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	loggingService := domainServices.Logging()

	return NewWorker(Config{
		LokiConfigService: loggingService,
		WriteLokiConfig: func(cfg logging.LokiConfig) error {
			return controllerruntimeconfig.ChangeControllerRuntimeConfig(
				c.RuntimeConfigPath,
				func(rtCfg *controllerruntimeconfig.ControllerRuntimeConfig) error {
					rtCfg.LokiEndpoint = cfg.Endpoint
					rtCfg.LokiCACert = cfg.CACertificate
					rtCfg.LokiInsecureSkipVerify = cfg.InsecureSkipVerify
					rtCfg.LokiOrgID = cfg.OrgID
					return nil
				},
			)
		},
		NotifyConfigReload: func() error {
			return controllerruntimeconfig.RequestReload(c.ConfigChangeSocketPath)
		},
		Logger: c.Logger,
	})
}

// getControllerDomainServices retrieves the controller domain services from
// the dependency getter.
func getControllerDomainServices(getter dependency.Getter, name string) (services.ControllerDomainServices, error) {
	var domainServices services.ControllerDomainServices
	if err := getter.Get(name, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices, nil
}
