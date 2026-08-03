// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/models"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/apicaller"
)

// ManifoldConfig describes the resources used by the firewaller worker.
type ManifoldConfig struct {
	DomainServicesName string
	EnvironName        string
	ModelUUID          string
	Logger             logger.Logger

	NewControllerConnection apicaller.NewExternalControllerConnectionFunc
	NewFirewallerWorker     func(Config) (worker.Worker, error)
}

// Manifold returns a Manifold that encapsulates the firewaller worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.EnvironName,
			cfg.DomainServicesName,
		},
		Start: cfg.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.EnvironName == "" {
		return errors.NotValidf("empty EnvironName")
	}
	if cfg.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewControllerConnection == nil {
		return errors.NotValidf("nil NewControllerConnection")
	}
	if cfg.NewFirewallerWorker == nil {
		return errors.NotValidf("nil NewFirewallerWorker")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var environ environs.Environ
	if err := getter.Get(cfg.EnvironName, &environ); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.DomainServices
	if err := getter.Get(cfg.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	// Check if the env supports global firewalling.  If the
	// configured mode is instance, we can ignore fwEnv being a
	// nil value, as it won't be used.
	fwEnv, fwEnvOK := environ.(environs.Firewaller)

	modelFw, _ := environ.(models.ModelFirewaller)

	mode := environ.Config().FirewallMode()
	if mode == config.FwNone {
		cfg.Logger.Infof(ctx, "stopping firewaller (not required)")
		return nil, dependency.ErrUninstall
	} else if mode == config.FwGlobal {
		if !fwEnvOK {
			cfg.Logger.Infof(ctx, "Firewall global mode set on provider with no support. stopping firewaller")
			return nil, dependency.ErrUninstall
		}
	}

	firewallerAPI := &firewallerAPIAdapter{
		machineSvc:       domainServices.Machine(),
		modelConfigSvc:   domainServices.Config(),
		ctrlConfigSvc:    domainServices.ControllerConfig(),
		networkSvc:       domainServices.Network(),
		relationSvc:      domainServices.Relation(),
		extControllerSvc: domainServices.ExternalController(),
		appSvc:           domainServices.Application(),
		modelInfoSvc:     domainServices.ModelInfo(),
	}

	// Check if the env supports IPV6 CIDRs for firewall ingress rules.
	var envIPV6CIDRSupport bool
	if featQuerier, ok := environ.(environs.FirewallFeatureQuerier); ok {
		var err error
		if envIPV6CIDRSupport, err = featQuerier.SupportsRulesWithIPV6CIDRs(ctx); err != nil {
			return nil, errors.Trace(err)
		}
	}

	w, err := cfg.NewFirewallerWorker(Config{
		ModelUUID:                 cfg.ModelUUID,
		CrossModelRelationService: domainServices.CrossModelRelation(),
		FirewallerAPI:             firewallerAPI,
		PortsService:              domainServices.Port(),
		ApplicationService:        domainServices.Application(),
		RelationService:           domainServices.Relation(),
		EnvironFirewaller:         fwEnv,
		EnvironModelFirewaller:    modelFw,
		EnvironInstances:          environ,
		EnvironIPV6CIDRSupport:    envIPV6CIDRSupport,
		Mode:                      mode,
		NewCrossModelFacadeFunc:   crossmodelFirewallerFacadeFunc(cfg.NewControllerConnection),
		Logger:                    cfg.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
