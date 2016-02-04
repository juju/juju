// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
)

var lxdLogger = loggo.GetLogger("juju.provisioner.lxd")

var _ environs.InstanceBroker = (*lxdBroker)(nil)

func NewLxdBroker(
	api APICalls,
	agentConfig agent.Config,
	managerConfig container.ManagerConfig,
	enableNAT bool,
) (environs.InstanceBroker, error) {
	namespace := maybeGetManagerConfigNamespaces(managerConfig)
	manager, err := lxd.NewContainerManager(managerConfig)
	if err != nil {
		return nil, err
	}

	return &lxdBroker{
		manager,
		namespace,
		api,
		agentConfig,
		enableNAT,
	}, nil
}

type lxdBroker struct {
	manager     container.Manager
	namespace   string
	api         APICalls
	agentConfig agent.Config
	enableNAT   bool
}

func (broker *lxdBroker) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	if args.InstanceConfig.HasNetworks() {
		return nil, errors.New("starting lxd containers with networks is not supported yet")
	}
	machineId := args.InstanceConfig.MachineId
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = lxd.DefaultLxdBridge
	}

	preparedInfo, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		machineId,
		bridgeDevice,
		true, // allocate if possible, do not maintain existing.
		broker.enableNAT,
		args.NetworkInfo,
		lxdLogger,
	)
	if err != nil {
		// It's not fatal (yet) if we couldn't pre-allocate addresses for the
		// container.
		logger.Warningf("failed to prepare container %q network config: %v", machineId, err)
	} else {
		args.NetworkInfo = preparedInfo
	}

	network := container.BridgeNetworkConfig(bridgeDevice, 0, args.NetworkInfo)

	series := args.Tools.OneSeries()
	args.InstanceConfig.MachineContainerType = instance.LXD
	args.InstanceConfig.Tools = args.Tools[0]

	config, err := broker.api.ContainerConfig()
	if err != nil {
		lxdLogger.Errorf("failed to get container config: %v", err)
		return nil, err
	}

	if err := instancecfg.PopulateInstanceConfig(
		args.InstanceConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		config.Proxy,
		config.AptProxy,
		config.AptMirror,
		config.PreferIPv6,
		config.EnableOSRefreshUpdate,
		config.EnableOSUpgrade,
	); err != nil {
		lxdLogger.Errorf("failed to populate machine config: %v", err)
		return nil, err
	}

	storageConfig := &container.StorageConfig{}
	inst, hardware, err := broker.manager.CreateContainer(args.InstanceConfig, series, network, storageConfig)
	if err != nil {
		return nil, err
	}

	return &environs.StartInstanceResult{
		Instance:    inst,
		Hardware:    hardware,
		NetworkInfo: network.Interfaces,
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
		maybeReleaseContainerAddresses(broker.api, id, broker.namespace, lxdLogger)
	}
	return nil
}

// AllInstances only returns running containers.
func (broker *lxdBroker) AllInstances() (result []instance.Instance, err error) {
	return broker.manager.ListContainers()
}

// MaintainInstance ensures the container's host has the required iptables and
// routing rules to make the container visible to both the host and other
// machines on the same subnet. This is important mostly when address allocation
// feature flag is enabled, as otherwise we don't create additional iptables
// rules or routes.
func (broker *lxdBroker) MaintainInstance(args environs.StartInstanceParams) error {
	machineID := args.InstanceConfig.MachineId

	// Default to using the host network until we can configure.
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = lxd.DefaultLxdBridge
	}

	// There's no InterfaceInfo we expect to get below.
	_, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		machineID,
		bridgeDevice,
		false, // maintain, do not allocate.
		broker.enableNAT,
		args.NetworkInfo,
		lxdLogger,
	)
	return err
}
