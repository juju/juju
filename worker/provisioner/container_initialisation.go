// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/common"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/broker"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	workercommon "github.com/juju/juju/worker/common"
)

// ContainerSetup is a StringsWatchHandler that is notified when containers
// are created on the given machine. It will set up the machine to be able
// to create containers and start a suitable provisioner.
type ContainerSetup struct {
	runner              *worker.Runner
	logger              Logger
	supportedContainers []instance.ContainerType
	provisioner         *apiprovisioner.State
	machine             apiprovisioner.MachineProvisioner
	config              agent.Config
	machineLock         machinelock.Lock

	// Save the workerName so the worker thread can be stopped.
	workerName string
	// setupDone[containerType] is non zero if the container setup has been
	// invoked for that container type.
	setupDone map[instance.ContainerType]*int32
	// The number of provisioners started. Once all necessary provisioners have
	// been started, the container watcher can be stopped.
	numberProvisioners int32
	credentialAPI      workercommon.CredentialAPI
	getNetConfig       func(common.NetworkConfigSource) ([]params.NetworkConfig, error)
}

// ContainerSetupParams are used to initialise a container setup handler.
type ContainerSetupParams struct {
	Runner              *worker.Runner
	Logger              Logger
	WorkerName          string
	SupportedContainers []instance.ContainerType
	Machine             apiprovisioner.MachineProvisioner
	Provisioner         *apiprovisioner.State
	Config              agent.Config
	MachineLock         machinelock.Lock
	CredentialAPI       workercommon.CredentialAPI
}

// NewContainerSetupHandler returns a StringsWatchHandler which is notified
// when containers are created on the given machine.
func NewContainerSetupHandler(params ContainerSetupParams) watcher.StringsHandler {
	return &ContainerSetup{
		runner:              params.Runner,
		logger:              params.Logger,
		machine:             params.Machine,
		supportedContainers: params.SupportedContainers,
		provisioner:         params.Provisioner,
		config:              params.Config,
		workerName:          params.WorkerName,
		machineLock:         params.MachineLock,
		credentialAPI:       params.CredentialAPI,
		getNetConfig:        common.GetObservedNetworkConfig,
	}
}

// SetUp is defined on the StringsWatchHandler interface.
func (cs *ContainerSetup) SetUp() (watcher watcher.StringsWatcher, err error) {
	// Set up the semaphores for each container type.
	cs.setupDone = make(map[instance.ContainerType]*int32, len(instance.ContainerTypes))
	for _, containerType := range instance.ContainerTypes {
		zero := int32(0)
		cs.setupDone[containerType] = &zero
	}
	// Listen to all container lifecycle events on our machine.
	if watcher, err = cs.machine.WatchAllContainers(); err != nil {
		return nil, err
	}
	return watcher, nil
}

// Handle is called whenever containers change on the machine being watched.
// Machines start out with no containers so the first time Handle is called,
// it will be because a container has been added.
func (cs *ContainerSetup) Handle(abort <-chan struct{}, containerIds []string) (resultError error) {
	// Consume the initial watcher event.
	if len(containerIds) == 0 {
		return nil
	}

	cs.logger.Infof("initial container setup with ids: %v", containerIds)
	for _, id := range containerIds {
		containerType := state.ContainerTypeFromId(id)
		// If this container type has been dealt with, do nothing.
		if atomic.LoadInt32(cs.setupDone[containerType]) != 0 {
			continue
		}
		if err := cs.initialiseAndStartProvisioner(abort, containerType); err != nil {
			cs.logger.Errorf("starting container provisioner for %v: %v", containerType, err)
			// Just because dealing with one type of container fails, we won't
			// exit the entire function because we still want to try and start
			// other container types. So we take note of and return the first
			// such error.
			if resultError == nil {
				resultError = err
			}
		}
	}
	return errors.Trace(resultError)
}

func (cs *ContainerSetup) initialiseAndStartProvisioner(
	abort <-chan struct{}, containerType instance.ContainerType,
) (resultError error) {
	// Flag that this container type has been handled.
	atomic.StoreInt32(cs.setupDone[containerType], 1)

	defer func() {
		if resultError != nil {
			cs.logger.Warningf("not stopping machine agent container watcher due to error: %v", resultError)
			return
		}
		if atomic.AddInt32(&cs.numberProvisioners, 1) == int32(len(cs.supportedContainers)) {
			// We only care about the initial container creation.
			// This worker has done its job so stop it.
			// We do not expect there will be an error, and there's not much we can do anyway.
			if err := cs.runner.StopWorker(cs.workerName); err != nil {
				cs.logger.Warningf("stopping machine agent container watcher: %v", err)
			}
		}
	}()

	cs.logger.Debugf("setup and start provisioner for %s containers", containerType)

	// Do an early check.
	if containerType != instance.LXD && containerType != instance.KVM {
		return fmt.Errorf("unknown container type: %v", containerType)
	}

	// Get the container manager config before other initialisation,
	// so we know if there are issues with host machine config.
	managerConfig, err := containerManagerConfig(containerType, cs.provisioner)
	if err != nil {
		return errors.Annotate(err, "generating container manager config")
	}
	managerConfigWithZones, err := broker.ConfigureAvailabilityZone(managerConfig, cs.machine)
	if err != nil {
		return errors.Annotate(err, "configuring availability zones")
	}

	if err := cs.initContainerDependencies(abort, containerType, managerConfig); err != nil {
		return errors.Annotate(err, "setting up container dependencies on host machine")
	}

	instanceBroker, err := broker.New(broker.Config{
		Name:          "provisioner",
		ContainerType: containerType,
		ManagerConfig: managerConfigWithZones,
		APICaller:     cs.provisioner,
		AgentConfig:   cs.config,
		MachineTag:    cs.machine.MachineTag(),
		MachineLock:   cs.machineLock,
		GetNetConfig:  cs.getNetConfig,
	})
	if err != nil {
		return errors.Annotate(err, "initialising container infrastructure on host machine")
	}

	toolsFinder := getToolsFinder(cs.provisioner)
	return StartProvisioner(
		cs.runner,
		containerType,
		cs.provisioner,
		cs.config,
		// Container provisioners are always good using the global logging
		// context.
		loggo.GetLogger("juju.worker.provisioner"),
		instanceBroker,
		toolsFinder,
		getDistributionGroupFinder(cs.provisioner),
		cs.credentialAPI,
	)
}

// initContainerDependencies ensures that the host machine is set-up to manage
// containers of the input type.
func (cs *ContainerSetup) initContainerDependencies(abort <-chan struct{}, containerType instance.ContainerType, managerCfg container.ManagerConfig) error {
	snapChannels := map[string]string{
		"lxd": managerCfg.PopValue(config.LXDSnapChannel),
	}
	initialiser := getContainerInitialiser(containerType, snapChannels)

	releaser, err := cs.acquireLock(fmt.Sprintf("%s container initialisation", containerType), abort)
	if err != nil {
		return errors.Annotate(err, "failed to acquire initialization lock")
	}
	defer releaser()

	if err := initialiser.Initialise(); err != nil {
		return errors.Trace(err)
	}

	// At this point, Initialiser likely has changed host network information,
	// so re-probe to have an accurate view.
	observedConfig, err := cs.observeNetwork()
	if err != nil {
		return errors.Annotate(err, "cannot discover observed network config")
	}
	if len(observedConfig) > 0 {
		machineTag := cs.machine.MachineTag()
		cs.logger.Tracef("updating observed network config for %q %s containers to %#v",
			machineTag, containerType, observedConfig)
		if err := cs.provisioner.SetHostMachineNetworkConfig(machineTag, observedConfig); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (cs *ContainerSetup) observeNetwork() ([]params.NetworkConfig, error) {
	return cs.getNetConfig(common.DefaultNetworkConfigSource())
}

func (cs *ContainerSetup) acquireLock(comment string, abort <-chan struct{}) (func(), error) {
	spec := machinelock.Spec{
		Cancel:  abort,
		Worker:  "provisioner",
		Comment: comment,
	}
	return cs.machineLock.Acquire(spec)
}

// TearDown is defined on the StringsWatchHandler interface. NoOp here.
func (cs *ContainerSetup) TearDown() error {
	return nil
}

// getContainerInitialiser exists to patch out in tests.
var getContainerInitialiser = func(ct instance.ContainerType, snapChannels map[string]string) container.Initialiser {
	if ct == instance.LXD {
		return lxd.NewContainerInitialiser(snapChannels["lxd"])
	}
	return kvm.NewContainerInitialiser()
}

func containerManagerConfig(
	containerType instance.ContainerType, provisioner *apiprovisioner.State,
) (container.ManagerConfig, error) {
	// Ask the provisioner for the container manager configuration.
	managerConfigResult, err := provisioner.ContainerManagerConfig(
		params.ContainerManagerConfigParams{Type: containerType},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	managerConfig := container.ManagerConfig(managerConfigResult.ManagerConfig)
	return managerConfig, nil
}

// Override for testing.
var StartProvisioner = startProvisionerWorker

// startProvisionerWorker kicks off a provisioner task responsible for creating
// containers of the specified type on the machine.
func startProvisionerWorker(
	runner *worker.Runner,
	containerType instance.ContainerType,
	provisioner *apiprovisioner.State,
	config agent.Config,
	logger Logger,
	broker environs.InstanceBroker,
	toolsFinder ToolsFinder,
	distributionGroupFinder DistributionGroupFinder,
	credentialAPI workercommon.CredentialAPI,
) error {

	workerName := fmt.Sprintf("%s-provisioner", containerType)
	// The provisioner task is created after a container record has
	// already been added to the machine. It will see that the
	// container does not have an instance yet and create one.
	return runner.StartWorker(workerName, func() (worker.Worker, error) {
		w, err := NewContainerProvisioner(containerType,
			provisioner,
			logger,
			config,
			broker,
			toolsFinder,
			distributionGroupFinder,
			credentialAPI,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return w, nil
	})
}
