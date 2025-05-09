// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner

import (
	"context"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provisionertask"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// Ensure our structs implement the required Provisioner interface.
var _ Provisioner = (*containerProvisioner)(nil)

var (
	retryStrategyDelay = 10 * time.Second
	retryStrategyCount = 10
)

// Provisioner represents a running provisioner worker.
type Provisioner interface {
	worker.Worker
}

// ControllerAPI describes API methods for querying a controller.
type ControllerAPI interface {
	ControllerConfig(context.Context) (controller.Config, error)
	CACert(context.Context) (string, error)
	ModelUUID(context.Context) (string, error)
	ModelConfig(context.Context) (*config.Config, error)
	WatchForModelConfigChanges(context.Context) (watcher.NotifyWatcher, error)
	APIAddresses(context.Context) ([]string, error)
}

// MachinesAPI describes API methods required to access machine provisioning info.
type MachinesAPI interface {
	Machines(context.Context, ...names.MachineTag) ([]apiprovisioner.MachineResult, error)
	MachinesWithTransientErrors(context.Context) ([]apiprovisioner.MachineStatusResult, error)
	WatchMachineErrorRetry(context.Context) (watcher.NotifyWatcher, error)
	WatchModelMachines(context.Context) (watcher.StringsWatcher, error)
	ProvisioningInfo(_ context.Context, machineTags []names.MachineTag) (params.ProvisioningInfoResults, error)
}

// ToolsFinder is an interface used for finding tools to run on
// provisioned instances.
type ToolsFinder interface {
	// FindTools returns a list of tools matching the specified
	// version, os, and architecture. If arch is empty, the
	// implementation is expected to use a well documented default.
	FindTools(ctx context.Context, version semversion.Number, os string, arch string) (coretools.List, error)
}

// DistributionGroupFinder provides access to machine distribution groups.
type DistributionGroupFinder interface {
	DistributionGroupByMachineId(context.Context, ...names.MachineTag) ([]apiprovisioner.DistributionGroupResult, error)
}

// containerProvisioner represents a running provisioning worker for containers
// hosted on a machine.
type containerProvisioner struct {
	containerType           instance.ContainerType
	machine                 apiprovisioner.MachineProvisioner
	configObserver          configObserver
	distributionGroupFinder DistributionGroupFinder
	toolsFinder             ToolsFinder
	controllerAPI           ControllerAPI
	machinesAPI             MachinesAPI
	agentConfig             agent.Config
	logger                  logger.Logger
	broker                  environs.InstanceBroker
	catacomb                catacomb.Catacomb
}

// configObserver is implemented so that tests can see when the environment
// configuration changes.
// The catacomb is set in export_test to the provider's member.
// This is used to prevent notify from blocking a provisioner that has had its
// Kill method invoked.
type configObserver struct {
	sync.Mutex
	observer chan<- *config.Config
	catacomb *catacomb.Catacomb
}

// notify notifies the observer of a configuration change.
func (o *configObserver) notify(cfg *config.Config) {
	o.Lock()
	if o.observer != nil {
		select {
		case o.observer <- cfg:
		case <-o.catacomb.Dying():
		}
	}
	o.Unlock()
}

// NewContainerProvisioner returns a new Provisioner. When new machines
// are added to the state, it allocates instances from the environment
// and allocates them to the new machines.
func NewContainerProvisioner(
	containerType instance.ContainerType,
	controllerAPI ControllerAPI,
	machinesAPI MachinesAPI,
	logger logger.Logger,
	agentConfig agent.Config,
	broker environs.InstanceBroker,
	toolsFinder ToolsFinder,
	distributionGroupFinder DistributionGroupFinder,
) (Provisioner, error) {
	p := &containerProvisioner{
		agentConfig:             agentConfig,
		logger:                  logger,
		controllerAPI:           controllerAPI,
		machinesAPI:             machinesAPI,
		broker:                  broker,
		containerType:           containerType,
		distributionGroupFinder: distributionGroupFinder,
		toolsFinder:             toolsFinder,
	}

	err := catacomb.Invoke(catacomb.Plan{
		Name: "container-provisioner",
		Site: &p.catacomb,
		Work: p.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p, nil
}

// Kill implements worker.Worker.Kill.
func (p *containerProvisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (p *containerProvisioner) Wait() error {
	return p.catacomb.Wait()
}

func (p *containerProvisioner) loop() error {
	ctx, cancel := p.scopedContext()
	defer cancel()

	p.logger.Tracef(ctx, "Starting %s provisioner for %q", p.containerType, p.agentConfig.Tag())

	modelWatcher, err := p.controllerAPI.WatchForModelConfigChanges(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(modelWatcher); err != nil {
		return errors.Trace(err)
	}

	modelConfig, err := p.controllerAPI.ModelConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	p.configObserver.notify(modelConfig)
	harvestMode := modelConfig.ProvisionerHarvestMode()
	workerCount := modelConfig.NumContainerProvisionWorkers()

	task, err := p.getStartTask(ctx, harvestMode, workerCount)
	if err != nil {
		return loggedErrorStack(p.logger, errors.Trace(err))
	}
	if err := p.catacomb.Add(task); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case _, ok := <-modelWatcher.Changes():
			if !ok {
				return errors.New("model configuration watch closed")
			}
			modelConfig, err := p.controllerAPI.ModelConfig(ctx)
			if err != nil {
				return errors.Annotate(err, "cannot load model configuration")
			}
			p.configObserver.notify(modelConfig)
			task.SetHarvestMode(modelConfig.ProvisionerHarvestMode())
			task.SetNumProvisionWorkers(modelConfig.NumContainerProvisionWorkers())
		}
	}
}

func (p *containerProvisioner) getMachine(ctx context.Context) (apiprovisioner.MachineProvisioner, error) {
	if p.machine == nil {
		tag := p.agentConfig.Tag()
		machineTag, ok := tag.(names.MachineTag)
		if !ok {
			return nil, errors.Errorf("expected names.MachineTag, got %T", tag)
		}
		result, err := p.machinesAPI.Machines(ctx, machineTag)
		if err != nil {
			p.logger.Errorf(ctx, "error retrieving %s from state", machineTag)
			return nil, err
		}
		if result[0].Err != nil {
			p.logger.Errorf(ctx, "%s is not in state", machineTag)
			return nil, err
		}
		p.machine = result[0].Machine
	}
	return p.machine, nil
}

func (p *containerProvisioner) getMachineWatcher(ctx context.Context) (watcher.StringsWatcher, error) {
	machine, err := p.getMachine(ctx)
	if err != nil {
		return nil, err
	}
	return machine.WatchContainers(ctx, p.containerType)
}

func (p *containerProvisioner) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(p.catacomb.Context(context.Background()))
}

// getStartTask creates a new worker for the provisioner,
func (p *containerProvisioner) getStartTask(ctx context.Context, harvestMode config.HarvestMode, workerCount int) (provisionertask.ProvisionerTask, error) {
	// Start responding to changes in machines, and to any further updates
	// to the environment config.
	machineWatcher, err := p.getMachineWatcher(ctx)
	if err != nil {
		return nil, err
	}
	hostTag := p.agentConfig.Tag()
	if kind := hostTag.Kind(); kind != names.ControllerAgentTagKind && kind != names.MachineTagKind {
		return nil, errors.Errorf("agent's tag is not a machine or controller agent tag, got %T", hostTag)
	}

	modelCfg, err := p.controllerAPI.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve the model config.")
	}

	controllerCfg, err := p.controllerAPI.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve the controller config.")
	}

	task, err := provisionertask.NewProvisionerTask(provisionertask.TaskConfig{
		ControllerUUID:               controllerCfg.ControllerUUID(),
		HostTag:                      hostTag,
		Logger:                       p.logger,
		HarvestMode:                  harvestMode,
		ControllerAPI:                p.controllerAPI,
		MachinesAPI:                  p.machinesAPI,
		GetMachineInstanceInfoSetter: machineInstanceInfoSetter,
		DistributionGroupFinder:      p.distributionGroupFinder,
		ToolsFinder:                  p.toolsFinder,
		MachineWatcher:               machineWatcher,
		Broker:                       p.broker,
		ImageStream:                  modelCfg.ImageStream(),
		RetryStartInstanceStrategy: provisionertask.RetryStrategy{
			RetryDelay: retryStrategyDelay,
			RetryCount: retryStrategyCount,
		},
		NumProvisionWorkers: workerCount, // event callback is currently only being used by tests
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return task, nil
}

// machineInstanceInfoSetter provides the mechanism for setting instance info
// for compute (machine) resources.
// This implementation uses the machines API, because the container
// provisioner is used on the agents where we cannot use domain services.
func machineInstanceInfoSetter(machineProvisionerAPI apiprovisioner.MachineProvisioner) func(
	ctx context.Context,
	id instance.Id, displayName string, nonce string, characteristics *instance.HardwareCharacteristics,
	networkConfig []params.NetworkConfig, volumes []params.Volume,
	volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
) error {
	return machineProvisionerAPI.SetInstanceInfo
}
