// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

var lxdLogger = loggo.GetLogger("juju.provisioner.lxd")

type PrepareHostFunc func(containerTag names.MachineTag, log loggo.Logger, abort <-chan struct{}) error

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

func (broker *lxdBroker) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	containerMachineID := args.InstanceConfig.MachineId

	config, err := broker.api.ContainerConfig()
	if err != nil {
		lxdLogger.Errorf("failed to get container config: %v", err)
		return nil, err
	}

	err = broker.prepareHost(names.NewMachineTag(containerMachineID), lxdLogger, args.Abort)
	if err != nil {
		return nil, errors.Trace(err)
	}

	preparedInfo, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		containerMachineID,
		true, // allocate if possible, do not maintain existing.
		lxdLogger,
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
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = network.DefaultLXDBridge
	}
	interfaces, err := finishNetworkConfig(bridgeDevice, preparedInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	net := container.BridgeNetworkConfig(bridgeDevice, 0, interfaces)

	pNames, err := broker.writeProfiles(containerMachineID)

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

	cloudInitUserData, err := combinedCloudInitData(
		config.CloudInitUserData,
		config.ContainerInheritProperties,
		series, lxdLogger)
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
		append([]string{"default"}, pNames...),
	); err != nil {
		lxdLogger.Errorf("failed to populate machine config: %v", err)
		return nil, err
	}

	storageConfig := &container.StorageConfig{}
	inst, hardware, err := broker.manager.CreateContainer(
		args.InstanceConfig, args.Constraints, series, net, storageConfig, args.StatusCallback,
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

func (broker *lxdBroker) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
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
func (broker *lxdBroker) AllInstances(ctx context.ProviderCallContext) (result []instance.Instance, err error) {
	return broker.manager.ListContainers()
}

// MaintainInstance ensures the container's host has the required iptables and
// routing rules to make the container visible to both the host and other
// machines on the same subnet.
func (broker *lxdBroker) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	machineID := args.InstanceConfig.MachineId

	// There's no InterfaceInfo we expect to get below.
	_, err := prepareOrGetContainerInterfaceInfo(
		broker.api,
		machineID,
		false, // maintain, do not allocate.
		lxdLogger,
	)
	return err
}

// LXDProfileNames returns all the profiles for a container that the broker
// currently is aware of.
func (broker *lxdBroker) LXDProfileNames(containerName string) ([]string, error) {
	nameRetriever, ok := broker.manager.(container.LXDProfileNameRetriever)
	if !ok {
		return make([]string, 0), nil
	}
	return nameRetriever.LXDProfileNames(containerName)
}

func (broker *lxdBroker) writeProfiles(machineID string) ([]string, error) {
	containerTag := names.NewMachineTag(machineID)
	profileInfo, err := broker.api.GetContainerProfileInfo(containerTag)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(profileInfo))
	for i, profile := range profileInfo {
		err := broker.maybeWriteLXDProfile(profile.Name, &charm.LXDProfile{
			Config:      profile.Config,
			Description: profile.Description,
			Devices:     profile.Devices,
		})
		if err != nil {
			return nil, err
		}
		names[i] = profile.Name
	}
	return names, nil
}

func (broker *lxdBroker) maybeWriteLXDProfile(pName string, put *charm.LXDProfile) error {
	profileMgr, ok := broker.manager.(container.LXDProfileManager)
	if !ok {
		return nil
	}
	return profileMgr.MaybeWriteLXDProfile(pName, put)
}
