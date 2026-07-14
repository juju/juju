// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/proxy"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	proxyupdaterapi "github.com/juju/juju/api/agent/proxyupdater"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/services"
)

// GetControllerDomainServicesFunc extracts controller domain services from a
// dependency getter.
type GetControllerDomainServicesFunc func(dependency.Getter, string) (ControllerDomainServices, error)

// GetDomainServicesFunc extracts model domain services for the supplied model
// UUID from a dependency getter.
type GetDomainServicesFunc func(context.Context, dependency.Getter, string, coremodel.UUID) (DomainServices, error)

// WaitReady reports whether the initial controller proxy configuration has
// been applied.
type WaitReady interface {
	WaitReady() bool
}

type readyWorker struct {
	worker.Worker
}

// WaitReady reports that the initial proxy configuration has been applied.
func (readyWorker) WaitReady() bool {
	return true
}

// ControllerManifoldConfig defines a proxy updater manifold backed directly by
// domain services instead of the API facade.
type ControllerManifoldConfig struct {
	DomainServicesName          string
	Logger                      logger.Logger
	WorkerFunc                  func(Config) (worker.Worker, error)
	GetControllerDomainServices GetControllerDomainServicesFunc
	GetDomainServices           GetDomainServicesFunc
	SupportLegacyValues         bool
	ExternalUpdate              func(proxy.Settings) error
	InProcessUpdate             func(proxy.Settings) error
	RunFunc                     func(string, string, ...string) (string, error)
}

// Validate ensures that all the required fields have values.
func (c ControllerManifoldConfig) Validate() error {
	if c.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if c.WorkerFunc == nil {
		return errors.NotValidf("nil WorkerFunc")
	}
	if c.GetControllerDomainServices == nil {
		return errors.NotValidf("nil GetControllerDomainServices")
	}
	if c.GetDomainServices == nil {
		return errors.NotValidf("nil GetDomainServices")
	}
	if c.ExternalUpdate == nil {
		return errors.NotValidf("nil ExternalUpdate")
	}
	if c.InProcessUpdate == nil {
		return errors.NotValidf("nil InProcessUpdate")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// ControllerManifold returns a dependency manifold that runs a proxy updater
// worker using domain services directly. This is intended for controller
// agents, where the services are already local and calling the API server
// would be redundant.
func ControllerManifold(config ControllerManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Output: outputControllerManifold,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			controllerDomainServices, err := config.GetControllerDomainServices(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			controllerModel, err := controllerDomainServices.Model().ControllerModel(ctx)
			if err != nil {
				return nil, errors.Trace(err)
			}

			domainServices, err := config.GetDomainServices(ctx, getter, config.DomainServicesName, controllerModel.UUID)
			if err != nil {
				return nil, errors.Trace(err)
			}

			source := domainProxySource{
				modelConfigService:    domainServices.Config(),
				controllerNodeService: controllerDomainServices.ControllerNode(),
			}
			workerConfig := Config{
				SystemdFiles:        []string{"/etc/juju-proxy-systemd.conf"},
				EnvFiles:            []string{"/etc/juju-proxy.conf"},
				API:                 source,
				SupportLegacyValues: config.SupportLegacyValues,
				ExternalUpdate:      config.ExternalUpdate,
				InProcessUpdate:     config.InProcessUpdate,
				Logger:              config.Logger,
				RunFunc:             config.RunFunc,
			}

			initialConfig, err := source.ProxyConfig(ctx)
			if err != nil {
				return nil, errors.Trace(err)
			}
			initialWorker := &proxyWorker{first: true, config: workerConfig}
			initialWorker.handleProxyValues(ctx, initialConfig.LegacyProxy, initialConfig.JujuProxy)

			w, err := config.WorkerFunc(workerConfig)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return readyWorker{Worker: w}, nil
		},
	}
}

func outputControllerManifold(in worker.Worker, out any) error {
	w, ok := in.(WaitReady)
	if !ok {
		return errors.Errorf("expected input implementing WaitReady, got %T", in)
	}

	target, ok := out.(*WaitReady)
	if !ok {
		return errors.Errorf("expected output of WaitReady, got %T", out)
	}
	*target = w
	return nil
}

// ControllerDomainServices exposes controller services used by this worker.
type ControllerDomainServices interface {
	// Model returns the controller model service.
	Model() ModelService
	// ControllerNode returns the controller node service.
	ControllerNode() ControllerNodeService
}

// DomainServices exposes model services used by this worker.
type DomainServices interface {
	// Config returns the model config service.
	Config() ModelConfigService
}

// ModelService provides controller model information.
type ModelService interface {
	// ControllerModel returns the model used for housing the Juju controller.
	ControllerModel(context.Context) (coremodel.Model, error)
}

// ModelConfigService provides access to the model's configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for model config changes.
	Watch(context.Context) (watcher.StringsWatcher, error)
}

// ControllerNodeService provides API address information for no-proxy values.
type ControllerNodeService interface {
	// GetAllNoProxyAPIAddressesForAgents returns agent API addresses suitable for
	// no-proxy settings.
	GetAllNoProxyAPIAddressesForAgents(context.Context) (string, error)
	// WatchControllerAPIAddresses watches controller API address changes.
	WatchControllerAPIAddresses(context.Context) (watcher.NotifyWatcher, error)
}

type domainProxySource struct {
	modelConfigService    ModelConfigService
	controllerNodeService ControllerNodeService
}

// GetControllerDomainServices retrieves controller services from the
// dependency getter.
func GetControllerDomainServices(getter dependency.Getter, name string) (ControllerDomainServices, error) {
	return coredependency.GetDependencyByName(getter, name, func(s services.ControllerDomainServices) ControllerDomainServices {
		return controllerDomainServices{services: s}
	})
}

// GetDomainServices retrieves model services from the dependency getter.
func GetDomainServices(ctx context.Context, getter dependency.Getter, name string, modelUUID coremodel.UUID) (DomainServices, error) {
	domainServicesGetter, err := coredependency.GetDependencyByName(getter, name, func(s services.DomainServicesGetter) services.DomainServicesGetter {
		return s
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	domainServices, err := domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return domainServicesAdapter{services: domainServices}, nil
}

type controllerDomainServices struct {
	services services.ControllerDomainServices
}

// Model returns the controller model service.
func (s controllerDomainServices) Model() ModelService {
	return s.services.Model()
}

// ControllerNode returns the controller node service.
func (s controllerDomainServices) ControllerNode() ControllerNodeService {
	return s.services.ControllerNode()
}

type domainServicesAdapter struct {
	services services.DomainServices
}

// Config returns the model config service.
func (s domainServicesAdapter) Config() ModelConfigService {
	return s.services.Config()
}

// ProxyConfig returns the proxy settings for the current model.
func (s domainProxySource) ProxyConfig(ctx context.Context) (proxyupdaterapi.ProxyConfiguration, error) {
	modelConfig, err := s.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return proxyupdaterapi.ProxyConfiguration{}, errors.Trace(err)
	}

	proxyAddressPorts, err := s.controllerNodeService.GetAllNoProxyAPIAddressesForAgents(ctx)
	if err != nil {
		return proxyupdaterapi.ProxyConfiguration{}, errors.Trace(err)
	}

	jujuProxySettings := modelConfig.JujuProxySettings()
	legacyProxySettings := modelConfig.LegacyProxySettings()
	if jujuProxySettings.HasProxySet() {
		jujuProxySettings.AutoNoProxy = proxyAddressPorts
	} else {
		legacyProxySettings.AutoNoProxy = proxyAddressPorts
	}

	return proxyupdaterapi.ProxyConfiguration{
		LegacyProxy:              legacyProxySettings,
		JujuProxy:                jujuProxySettings,
		APTProxy:                 modelConfig.AptProxySettings(),
		SnapProxy:                modelConfig.SnapProxySettings(),
		AptMirror:                modelConfig.AptMirror(),
		SnapStoreProxyId:         modelConfig.SnapStoreProxy(),
		SnapStoreProxyAssertions: modelConfig.SnapStoreAssertions(),
		SnapStoreProxyURL:        modelConfig.SnapStoreProxyURL(),
	}, nil
}

// WatchForProxyConfigAndAPIHostPortChanges watches for proxy settings and API
// address changes.
func (s domainProxySource) WatchForProxyConfigAndAPIHostPortChanges(ctx context.Context) (watcher.NotifyWatcher, error) {
	modelConfigWatcher, err := s.modelConfigService.Watch(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelConfigNotifyWatcher, err := eventsource.NewStringsNotifyWatcher(modelConfigWatcher)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerAPIAddressesWatcher, err := s.controllerNodeService.WatchControllerAPIAddresses(ctx)
	if err != nil {
		modelConfigNotifyWatcher.Kill()
		return nil, errors.Trace(err)
	}

	return eventsource.NewMultiNotifyWatcher(
		ctx,
		modelConfigNotifyWatcher,
		controllerAPIAddressesWatcher,
	)
}

var _ API = domainProxySource{}
