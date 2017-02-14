// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mutex"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/common"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
)

var (
	systemNetworkInterfacesFile = "/etc/network/interfaces"
	activateBridgesTimeout      = 5 * time.Minute
)

// ContainerSetup is a StringsWatchHandler that is notified when containers
// are created on the given machine. It will set up the machine to be able
// to create containers and start a suitable provisioner.
type ContainerSetup struct {
	runner              worker.Runner
	supportedContainers []instance.ContainerType
	provisioner         *apiprovisioner.State
	machine             *apiprovisioner.Machine
	config              agent.Config
	initLockName        string

	// Save the workerName so the worker thread can be stopped.
	workerName string
	// setupDone[containerType] is non zero if the container setup has been invoked
	// for that container type.
	setupDone map[instance.ContainerType]*int32
	// The number of provisioners started. Once all necessary provisioners have
	// been started, the container watcher can be stopped.
	numberProvisioners int32
}

// ContainerSetupParams are used to initialise a container setup handler.
type ContainerSetupParams struct {
	Runner              worker.Runner
	WorkerName          string
	SupportedContainers []instance.ContainerType
	Machine             *apiprovisioner.Machine
	Provisioner         *apiprovisioner.State
	Config              agent.Config
	InitLockName        string
}

// NewContainerSetupHandler returns a StringsWatchHandler which is notified when
// containers are created on the given machine.
func NewContainerSetupHandler(params ContainerSetupParams) watcher.StringsHandler {
	return &ContainerSetup{
		runner:              params.Runner,
		machine:             params.Machine,
		supportedContainers: params.SupportedContainers,
		provisioner:         params.Provisioner,
		config:              params.Config,
		workerName:          params.WorkerName,
		initLockName:        params.InitLockName,
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

	logger.Infof("initial container setup with ids: %v", containerIds)
	for _, id := range containerIds {
		containerType := state.ContainerTypeFromId(id)
		// If this container type has been dealt with, do nothing.
		if atomic.LoadInt32(cs.setupDone[containerType]) != 0 {
			continue
		}
		if err := cs.initialiseAndStartProvisioner(abort, containerType); err != nil {
			logger.Errorf("starting container provisioner for %v: %v", containerType, err)
			// Just because dealing with one type of container fails, we won't exit the entire
			// function because we still want to try and start other container types. So we
			// take note of and return the first such error.
			if resultError == nil {
				resultError = err
			}
		}
	}
	return resultError
}

func (cs *ContainerSetup) initialiseAndStartProvisioner(abort <-chan struct{}, containerType instance.ContainerType) (resultError error) {
	// Flag that this container type has been handled.
	atomic.StoreInt32(cs.setupDone[containerType], 1)

	defer func() {
		if resultError != nil {
			logger.Warningf("not stopping machine agent container watcher due to error: %v", resultError)
			return
		}
		if atomic.AddInt32(&cs.numberProvisioners, 1) == int32(len(cs.supportedContainers)) {
			// We only care about the initial container creation.
			// This worker has done its job so stop it.
			// We do not expect there will be an error, and there's not much we can do anyway.
			if err := cs.runner.StopWorker(cs.workerName); err != nil {
				logger.Warningf("stopping machine agent container watcher: %v", err)
			}
		}
	}()

	logger.Debugf("setup and start provisioner for %s containers", containerType)
	toolsFinder := getToolsFinder(cs.provisioner)
	initialiser, broker, toolsFinder, err := cs.getContainerArtifacts(containerType, toolsFinder)
	if err != nil {
		return errors.Annotate(err, "initialising container infrastructure on host machine")
	}
	if err := cs.runInitialiser(abort, containerType, initialiser); err != nil {
		return errors.Annotate(err, "setting up container dependencies on host machine")
	}
	return StartProvisioner(cs.runner, containerType, cs.provisioner, cs.config, broker, toolsFinder)
}

// acquireLock tries to grab the machine lock (initLockName), and either
// returns it in a locked state, or returns an error.
func (cs *ContainerSetup) acquireLock(abort <-chan struct{}) (mutex.Releaser, error) {
	spec := mutex.Spec{
		Name:  cs.initLockName,
		Clock: clock.WallClock,
		// If we don't get the lock straight away, there is no point trying multiple
		// times per second for an operation that is likelty to take multiple seconds.
		Delay:  time.Second,
		Cancel: abort,
	}
	return mutex.Acquire(spec)
}

// runInitialiser runs the container initialiser with the initialisation hook held.
func (cs *ContainerSetup) runInitialiser(abort <-chan struct{}, containerType instance.ContainerType, initialiser container.Initialiser) error {
	logger.Debugf("running initialiser for %s containers, acquiring lock %q", containerType, cs.initLockName)
	releaser, err := cs.acquireLock(abort)
	if err != nil {
		return errors.Annotate(err, "failed to acquire initialization lock")
	}
	logger.Debugf("lock %q acquired", cs.initLockName)
	defer logger.Debugf("release lock %q for container initialisation", cs.initLockName)
	defer releaser.Release()

	if err := initialiser.Initialise(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// TearDown is defined on the StringsWatchHandler interface.
func (cs *ContainerSetup) TearDown() error {
	// Nothing to do here.
	return nil
}

// getObservedNetworkConfig is here to allow us to override it for testing.
// TODO(jam): Find a way to pass it into ContainerSetup instead of a global variable
var getObservedNetworkConfig = common.GetObservedNetworkConfig

func (cs *ContainerSetup) prepareHost(containerTag names.MachineTag, log loggo.Logger) error {
	devicesToBridge, reconfigureDelay, err := cs.provisioner.HostChangesForContainer(containerTag)
	if err != nil {
		return errors.Annotate(err, "unable to setup network")
	}

	if len(devicesToBridge) == 0 {
		log.Debugf("container %q requires no additional bridges", containerTag)
		return nil
	}

	bridger, err := network.DefaultEtcNetworkInterfacesBridger(activateBridgesTimeout, instancecfg.DefaultBridgePrefix, systemNetworkInterfacesFile)
	if err != nil {
		return errors.Trace(err)
	}

	log.Debugf("Bridging %+v devices on host %q for container %q with delay=%v, acquiring lock %q",
		devicesToBridge, cs.machine.MachineTag().String(), containerTag.String(), reconfigureDelay, cs.initLockName)
	// TODO(jam): 2017-02-08 figure out how to thread catacomb.Dying() into
	// this function, so that we can stop trying to acquire the lock if we are
	// stopping.
	releaser, err := cs.acquireLock(nil)
	if err != nil {
		return errors.Annotatef(err, "failed to acquire lock %q for bridging", cs.initLockName)
	}
	defer log.Debugf("releasing lock %q for bridging machine %q for container %q", cs.initLockName, cs.machine.MachineTag().String(), containerTag.String())
	defer releaser.Release()
	err = bridger.Bridge(devicesToBridge, reconfigureDelay)
	if err != nil {
		return errors.Annotate(err, "failed to bridge devices")
	}

	// We just changed the hosts' network setup so discover new
	// interfaces/devices and propagate to state.
	observedConfig, err := getObservedNetworkConfig(common.DefaultNetworkConfigSource())
	if err != nil {
		return errors.Annotate(err, "cannot discover observed network config")
	}

	if len(observedConfig) > 0 {
		err := cs.provisioner.SetHostMachineNetworkConfig(cs.machine.MachineTag(), observedConfig)
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("observed network config updated")
	}

	return nil
}

// getContainerArtifacts returns type-specific interfaces for
// managing containers.
//
// The ToolsFinder passed in may be replaced or wrapped to
// enforce container-specific constraints.
func (cs *ContainerSetup) getContainerArtifacts(
	containerType instance.ContainerType, toolsFinder ToolsFinder,
) (
	container.Initialiser,
	environs.InstanceBroker,
	ToolsFinder,
	error,
) {
	var initialiser container.Initialiser
	var broker environs.InstanceBroker

	managerConfig, err := containerManagerConfig(containerType, cs.provisioner, cs.config)
	if err != nil {
		return nil, nil, nil, err
	}

	switch containerType {
	case instance.KVM:
		initialiser = kvm.NewContainerInitialiser()
		manager, err := kvm.NewContainerManager(managerConfig)
		if err != nil {
			return nil, nil, nil, err
		}
		broker, err = NewKVMBroker(
			cs.prepareHost,
			cs.provisioner,
			manager,
			cs.config,
		)
		if err != nil {
			logger.Errorf("failed to create new kvm broker")
			return nil, nil, nil, err
		}
	case instance.LXD:
		series, err := cs.machine.Series()
		if err != nil {
			return nil, nil, nil, err
		}

		initialiser = lxd.NewContainerInitialiser(series)
		manager, err := lxd.NewContainerManager(managerConfig)
		if err != nil {
			return nil, nil, nil, err
		}
		broker, err = NewLXDBroker(
			cs.prepareHost,
			cs.provisioner,
			manager,
			cs.config,
		)
		if err != nil {
			logger.Errorf("failed to create new lxd broker")
			return nil, nil, nil, err
		}
	default:
		return nil, nil, nil, fmt.Errorf("unknown container type: %v", containerType)
	}

	return initialiser, broker, toolsFinder, nil
}

func containerManagerConfig(
	containerType instance.ContainerType,
	provisioner *apiprovisioner.State,
	agentConfig agent.Config,
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

// startProvisionerWorker kicks off a provisioner task responsible for creating containers
// of the specified type on the machine.
func startProvisionerWorker(
	runner worker.Runner,
	containerType instance.ContainerType,
	provisioner *apiprovisioner.State,
	config agent.Config,
	broker environs.InstanceBroker,
	toolsFinder ToolsFinder,
) error {

	workerName := fmt.Sprintf("%s-provisioner", containerType)
	// The provisioner task is created after a container record has
	// already been added to the machine. It will see that the
	// container does not have an instance yet and create one.
	return runner.StartWorker(workerName, func() (worker.Worker, error) {
		w, err := NewContainerProvisioner(containerType, provisioner, config, broker, toolsFinder)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return w, nil
	})
}
