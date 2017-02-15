// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

var lxdLogger = loggo.GetLogger("juju.provisioner.lxd")

type PrepareHostFunc func(containerTag names.MachineTag, log loggo.Logger) error

// TODO(jam): 2017-02-08 should be removed, only here for legacy testing, etc.
func NewLxdBroker(
	bridger network.Bridger,
	hostMachineTag names.MachineTag,
	api APICalls,
	manager container.Manager,
	agentConfig agent.Config,
) (environs.InstanceBroker, error) {
	prepareWrapper := func(containerTag names.MachineTag, log loggo.Logger) error {
		return prepareHost(bridger, hostMachineTag, containerTag, api, log)
	}
	return NewLXDBroker(prepareWrapper, api, manager, agentConfig)
}

// NewLXDBroker creates a Broker that can be used to start LXD containers in a
// similar fashion to normal StartInstance requests.
// prepareHost is a callback that will be called when a new container is about
// to be started. It provides the intersection point where the host can update
// itself to be ready for whatever changes are necessary to have a functioning
// container. (such as bridging host devices.)
// manager is the infrastructure to actually launch the container.
// agentConfig is currently only used to find out the 'default' bridge to use
// when a specific network device is not specified in StartInstanceParams. This
// should be deprecated. And hopefully removed in the future.
func NewLXDBroker(
	prepareHost PrepareHostFunc,
	api APICalls,
	manager container.Manager,
	agentConfig agent.Config,
) (environs.InstanceBroker, error) {
	return &lxdBroker{
		prepareHost: prepareHost,
		manager:     manager,
		api:         api,
		agentConfig: agentConfig,
	}, nil
}

type lxdBroker struct {
	prepareHost PrepareHostFunc
	manager     container.Manager
	api         APICalls
	agentConfig agent.Config
}

func (broker *lxdBroker) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	containerMachineID := args.InstanceConfig.MachineId
	bridgeDevice := broker.agentConfig.Value(agent.LxdBridge)
	if bridgeDevice == "" {
		bridgeDevice = network.DefaultLXDBridge
	}

	config, err := broker.api.ContainerConfig()
	if err != nil {
		lxdLogger.Errorf("failed to get container config: %v", err)
		return nil, err
	}

	err = broker.prepareHost(names.NewMachineTag(containerMachineID), lxdLogger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	preparedInfo, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		containerMachineID,
		bridgeDevice,
		true, // allocate if possible, do not maintain existing.
		lxdLogger,
	)
	if err != nil {
		// It's not fatal (yet) if we couldn't pre-allocate addresses for the
		// container.
		logger.Warningf("failed to prepare container %q network config: %v", containerMachineID, err)
	} else {
		args.NetworkInfo = preparedInfo
	}

	network := container.BridgeNetworkConfig(bridgeDevice, 0, args.NetworkInfo)
	interfaces, err := finishNetworkConfig(bridgeDevice, args.NetworkInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	network.Interfaces = interfaces

	// The provisioner worker will provide all tools it knows about
	// (after applying explicitly specified constraints), which may
	// include tools for architectures other than the host's. We
	// must constrain to the host's architecture for LXD.
	archTools, err := matchHostArchTools(args.Tools)
	if err != nil {
		return nil, errors.Trace(err)
	}

	series := archTools.OneSeries()
	args.InstanceConfig.MachineContainerType = instance.LXD
	if err := args.InstanceConfig.SetTools(archTools); err != nil {
		return nil, errors.Trace(err)
	}

	if err := instancecfg.PopulateInstanceConfig(
		args.InstanceConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		config.Proxy,
		config.AptProxy,
		config.AptMirror,
		config.EnableOSRefreshUpdate,
		config.EnableOSUpgrade,
	); err != nil {
		lxdLogger.Errorf("failed to populate machine config: %v", err)
		return nil, err
	}

	storageConfig := &container.StorageConfig{}
	inst, hardware, err := broker.manager.CreateContainer(
		args.InstanceConfig, args.Constraints,
		series, network, storageConfig, args.StatusCallback,
	)
	if err != nil {
		return nil, err
	}

	return &environs.StartInstanceResult{
		Instance:    inst,
		Hardware:    hardware,
		NetworkInfo: interfaces,
	}, nil
}

func (broker *lxdBroker) StopInstances(ids ...instance.Id) error {
	// TODO: potentially parallelise.
	for _, id := range ids {
		lxdLogger.Infof("stopping lxd container for instance: %s", id)
		if err := broker.manager.DestroyContainer(id); err != nil {
			lxdLogger.Errorf("container did not stop: %v", err)
			return err
		}
		releaseContainerAddresses(broker.api, id, broker.manager.Namespace(), lxdLogger)
	}
	return nil
}

// AllInstances only returns running containers.
func (broker *lxdBroker) AllInstances() (result []instance.Instance, err error) {
	return broker.manager.ListContainers()
}

// MaintainInstance ensures the container's host has the required iptables and
// routing rules to make the container visible to both the host and other
// machines on the same subnet.
func (broker *lxdBroker) MaintainInstance(args environs.StartInstanceParams) error {
	machineID := args.InstanceConfig.MachineId

	// Default to using the host network until we can configure.
	bridgeDevice := broker.agentConfig.Value(agent.LxdBridge)
	if bridgeDevice == "" {
		bridgeDevice = network.DefaultLXDBridge
	}

	// There's no InterfaceInfo we expect to get below.
	_, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		machineID,
		bridgeDevice,
		false, // maintain, do not allocate.
		lxdLogger,
	)
	return err
}
