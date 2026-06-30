// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner

import (
	"context"

	"github.com/juju/errors"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
)

// MachineService defines the methods that the worker assumes from the Machine
// service.
type MachineService interface {
	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(ctx context.Context, machineUUID coremachine.UUID, instanceID instance.Id, displayName, nonce string, hardwareCharacteristics *instance.HardwareCharacteristics) error
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name coremachine.Name) (coremachine.UUID, error)
}

// GetMachineServiceFunc is a helper function that gets a service from the manifold.
type GetMachineServiceFunc func(getter dependency.Getter, name string) (MachineService, error)

// GetMachineService is a helper function that gets a service from the
// manifold.
func GetMachineService(getter dependency.Getter, name string) (MachineService, error) {
	return coredependency.GetDependencyByName(getter, name, func(s services.ModelDomainServices) MachineService {
		return s.Machine()
	})
}

// DomainServices holds the subset of domain services needed by the
// compute-provisioner manifold. This avoids depending on the full
// services.DomainServices interface.
type DomainServices struct {
	controllerConfig ControllerConfigService
	modelConfig      ModelConfigService
	controllerNode   ControllerNodeService
	modelInfo        ModelInfoService
	machine          MachineDomainService
	status           StatusDomainService
	provisioning     ProvisionerDomainService
	agentBinary      AgentBinaryDomainService
	application      ApplicationDomainService
	agentPassword    AgentPasswordDomainService
	removal          RemovalDomainService
}

// GetDomainServicesFunc is a helper function that gets the domain services
// from the manifold.
type GetDomainServicesFunc func(getter dependency.Getter, name string) (DomainServices, error)

// GetDomainServices is a helper function that gets the domain services
// from the manifold.
func GetDomainServices(getter dependency.Getter, name string) (DomainServices, error) {
	return coredependency.GetDependencyByName(getter, name, func(s services.DomainServices) DomainServices {
		return DomainServices{
			controllerConfig: s.ControllerConfig(),
			modelConfig:      s.Config(),
			controllerNode:   s.ControllerNode(),
			modelInfo:        s.ModelInfo(),
			machine:          s.Machine(),
			status:           s.Status(),
			provisioning:     s.Provisioning(),
			agentBinary:      s.AgentBinary(),
			application:      s.Application(),
			agentPassword:    s.AgentPassword(),
			removal:          s.Removal(),
		}
	})
}

// ManifoldConfig defines an environment provisioner's dependencies. It's not
// currently clear whether it'll be easier to extend this type to include all
// provisioners, or to create separate (Environ|Container)Manifold[Config]s;
// for now we dodge the question because we don't need container provisioners
// in dependency engines. Yet.
type ManifoldConfig struct {
	EnvironName        string
	DomainServicesName string
	GetMachineService  GetMachineServiceFunc
	GetDomainServices  GetDomainServicesFunc
	Logger             logger.Logger
	AgentTag           names.Tag
	ModelUUID          string
	NewProvisionerFunc func(ControllerAPI, MachineService, MachinesAPI, ToolsFinder, DistributionGroupFinder, names.Tag, logger.Logger, Environ) (Provisioner, error)
}

// Manifold creates a manifold that runs an environment provisioner. See the
// ManifoldConfig type for discussion about how this can/should evolve.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.EnvironName,
			config.DomainServicesName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var environ environs.Environ
			if err := getter.Get(config.EnvironName, &environ); err != nil {
				return nil, errors.Trace(err)
			}

			domSvc, err := config.GetDomainServices(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			machineService, err := config.GetMachineService(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			controllerAPI := &controllerAPIAdapter{
				ctrlConfigSvc:  domSvc.controllerConfig,
				modelConfigSvc: domSvc.modelConfig,
				ctrlNodeSvc:    domSvc.controllerNode,
				modelUUID:      config.ModelUUID,
			}

			machinesAPI := &machinesAPIAdapter{
				machineSvc:       domSvc.machine,
				statusSvc:        domSvc.status,
				provisionerSvc:   domSvc.provisioning,
				ctrlConfigSvc:    domSvc.controllerConfig,
				modelConfigSvc:   domSvc.modelConfig,
				modelInfoSvc:     domSvc.modelInfo,
				agentPasswordSvc: domSvc.agentPassword,
				removalSvc:       domSvc.removal,
				machineService:   machineService,
				modelUUID:        config.ModelUUID,
			}

			toolsFinder := &toolsFinderAdapter{
				agentBinarySvc: domSvc.agentBinary,
				ctrlNodeSvc:    domSvc.controllerNode,
				modelUUID:      config.ModelUUID,
			}

			distGroupFinder := &distributionGroupFinderAdapter{
				machineSvc:     domSvc.machine,
				appSvc:         domSvc.application,
				machineService: machineService,
			}

			w, err := config.NewProvisionerFunc(controllerAPI, machineService, machinesAPI, toolsFinder, distGroupFinder, config.AgentTag, config.Logger, environ)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
		Filter: internalworker.ShouldWorkerUninstall,
	}
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.EnvironName == "" {
		return errors.NotValidf("empty EnvironName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.GetMachineService == nil {
		return errors.NotValidf("nil GetMachineService")
	}
	if config.GetDomainServices == nil {
		return errors.NotValidf("nil GetDomainServices")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.AgentTag == nil {
		return errors.NotValidf("nil AgentTag")
	}
	if config.NewProvisionerFunc == nil {
		return errors.NotValidf("nil NewProvisionerFunc")
	}
	return nil
}
