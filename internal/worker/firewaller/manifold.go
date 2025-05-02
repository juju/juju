// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/remoterelations"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/models"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/apicaller"
)

// ManifoldConfig describes the resources used by the firewaller worker.
//
// TODO(jack-w-shaw): This is a model worker, so domain services can be accessed
// directly instead of going via an API. However, not all dependencies are
// available as domain services, so we still need to use the API for some things.
// Once all dependencies are available as domain services, we can remove the
// APICaller.
type ManifoldConfig struct {
	AgentName          string
	APICallerName      string
	DomainServicesName string
	EnvironName        string
	Logger             logger.Logger

	NewControllerConnection  apicaller.NewExternalControllerConnectionFunc
	NewRemoteRelationsFacade func(base.APICaller) *remoterelations.Client
	NewFirewallerFacade      func(base.APICaller) (FirewallerAPI, error)
	NewFirewallerWorker      func(Config) (worker.Worker, error)
}

// Manifold returns a Manifold that encapsulates the firewaller worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.AgentName,
			cfg.APICallerName,
			cfg.EnvironName,
			cfg.DomainServicesName,
		},
		Start: cfg.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if cfg.EnvironName == "" {
		return errors.NotValidf("empty EnvironName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewControllerConnection == nil {
		return errors.NotValidf("nil NewControllerConnection")
	}
	if cfg.NewRemoteRelationsFacade == nil {
		return errors.NotValidf("nil NewRemoteRelationsFacade")
	}
	if cfg.NewFirewallerFacade == nil {
		return errors.NotValidf("nil NewFirewallerFacade")
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

	var agent agent.Agent
	if err := getter.Get(cfg.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}
	var apiConn api.Connection
	if err := getter.Get(cfg.APICallerName, &apiConn); err != nil {
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

	firewallerAPI, err := cfg.NewFirewallerFacade(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
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
		ModelUUID:               agent.CurrentConfig().Model().Id(),
		RemoteRelationsApi:      cfg.NewRemoteRelationsFacade(apiConn),
		FirewallerAPI:           firewallerAPI,
		PortsService:            domainServices.Port(),
		MachineService:          domainServices.Machine(),
		ApplicationService:      domainServices.Application(),
		EnvironFirewaller:       fwEnv,
		EnvironModelFirewaller:  modelFw,
		EnvironInstances:        environ,
		EnvironIPV6CIDRSupport:  envIPV6CIDRSupport,
		Mode:                    mode,
		NewCrossModelFacadeFunc: crossmodelFirewallerFacadeFunc(cfg.NewControllerConnection),
		Logger:                  cfg.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
