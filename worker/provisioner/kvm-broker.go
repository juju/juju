// Copyright 2013 Canonical Ltd.
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
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

var kvmLogger = loggo.GetLogger("juju.provisioner.kvm")

// NewKVMBroker creates a Broker that can be used to start KVM guests in a
// similar fashion to normal StartInstance requests.
// prepareHost is a callback that will be called when a new container is about
// to be started. It provides the intersection point where the host can update
// itself to be ready for whatever changes are necessary to have a functioning
// container. (such as bridging host devices.)
// manager is the infrastructure to actually launch the container.
// agentConfig is currently only used to find out the 'default' bridge to use
// when a specific network device is not specified in StartInstanceParams. This
// should be deprecated. And hopefully removed in the future.
func NewKVMBroker(
	prepareHost PrepareHostFunc,
	api APICalls,
	manager container.Manager,
	agentConfig agent.Config,
) (environs.InstanceBroker, error) {
	return &kvmBroker{
		prepareHost: prepareHost,
		manager:     manager,
		api:         api,
		agentConfig: agentConfig,
	}, nil
}

type kvmBroker struct {
	prepareHost PrepareHostFunc
	manager     container.Manager
	api         APICalls
	agentConfig agent.Config
}

// StartInstance is specified in the Broker interface.
func (broker *kvmBroker) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// TODO: refactor common code out of the container brokers.
	containerMachineID := args.InstanceConfig.MachineId
	kvmLogger.Infof("starting kvm container for containerMachineID: %s", containerMachineID)

	// TODO: Default to using the host network until we can configure.  Yes,
	// this is using the LxcBridge value, we should put it in the api call for
	// container config.
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = network.DefaultKVMBridge
	}

	config, err := broker.api.ContainerConfig()
	if err != nil {
		kvmLogger.Errorf("failed to get container config: %v", err)
		return nil, err
	}

	err = broker.prepareHost(names.NewMachineTag(containerMachineID), kvmLogger, args.Abort)
	if err != nil {
		return nil, errors.Trace(err)
	}

	preparedInfo, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		containerMachineID,
		true, // allocate if possible, do not maintain existing.
		kvmLogger,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Something to fallback to if there are no devices given in args.NetworkInfo
	// TODO(jam): 2017-02-07, this feels like something that should never need
	// to be invoked, because either StartInstance or
	// prepareOrGetContainerInterfaceInfo should always return a value. The
	// test suite currently doesn't think so, and I'm hesitant to munge it too
	// much.
	interfaces, err := finishNetworkConfig(bridgeDevice, preparedInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	network := container.BridgeNetworkConfig(bridgeDevice, 0, interfaces)

	// The provisioner worker will provide all tools it knows about
	// (after applying explicitly specified constraints), which may
	// include tools for architectures other than the host's.
	//
	// container/kvm only allows running container==host arch, so
	// we constrain the tools to host arch here regardless of the
	// constraints specified.
	archTools, err := matchHostArchTools(args.Tools)
	if err != nil {
		return nil, errors.Trace(err)
	}

	series := archTools.OneSeries()
	args.InstanceConfig.MachineContainerType = instance.KVM
	if err := args.InstanceConfig.SetTools(archTools); err != nil {
		return nil, errors.Trace(err)
	}

	cloudInitUserData, err := combinedCloudInitData(
		config.CloudInitUserData,
		config.ContainerInheritProperties,
		series, kvmLogger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := instancecfg.PopulateInstanceConfig(
		args.InstanceConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		config.LegacyProxy,
		config.JujuProxy,
		config.AptProxy,
		config.AptMirror,
		config.EnableOSRefreshUpdate,
		config.EnableOSUpgrade,
		cloudInitUserData,
		nil,
	); err != nil {
		kvmLogger.Errorf("failed to populate machine config: %v", err)
		return nil, err
	}

	storageConfig := &container.StorageConfig{
		AllowMount: true,
	}
	inst, hardware, err := broker.manager.CreateContainer(
		args.InstanceConfig, args.Constraints,
		series, network, storageConfig, args.StatusCallback,
	)
	if err != nil {
		kvmLogger.Errorf("failed to start container: %v", err)
		return nil, err
	}
	kvmLogger.Infof("started kvm container for containerMachineID: %s, %s, %s", containerMachineID, inst.Id(), hardware.String())
	return &environs.StartInstanceResult{
		Instance:    inst,
		Hardware:    hardware,
		NetworkInfo: interfaces,
	}, nil
}

// MaintainInstance ensures the container's host has the required iptables and
// routing rules to make the container visible to both the host and other
// machines on the same subnet.
func (broker *kvmBroker) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	machineID := args.InstanceConfig.MachineId

	// There's no InterfaceInfo we expect to get below.
	_, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		machineID,
		false, // maintain, do not allocate.
		kvmLogger,
	)
	return err
}

// StopInstances shuts down the given instances.
func (broker *kvmBroker) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	// TODO: potentially parallelise.
	for _, id := range ids {
		kvmLogger.Infof("stopping kvm container for instance: %s", id)
		if err := broker.manager.DestroyContainer(id); err != nil {
			kvmLogger.Errorf("container did not stop: %v", err)
			return err
		}
		releaseContainerAddresses(broker.api, id, broker.manager.Namespace(), kvmLogger)
	}
	return nil
}

// AllInstances only returns running containers.
func (broker *kvmBroker) AllInstances(ctx context.ProviderCallContext) (result []instance.Instance, err error) {
	return broker.manager.ListContainers()
}
