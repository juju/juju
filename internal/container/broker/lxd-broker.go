// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/container"
	internallogger "github.com/juju/juju/internal/logger"
)

var lxdLogger = internallogger.GetLogger("juju.container.broker.lxd")

type PrepareHostFunc func(ctx context.Context, containerTag names.MachineTag, log corelogger.Logger, abort <-chan struct{}) error

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

func (broker *lxdBroker) StartInstance(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	containerMachineID := args.InstanceConfig.MachineId

	config, err := broker.api.ContainerConfig(ctx)
	if err != nil {
		lxdLogger.Errorf(context.TODO(), "failed to get container config: %v", err)
		return nil, err
	}

	if err := broker.prepareHost(ctx, names.NewMachineTag(containerMachineID), lxdLogger, args.Abort); err != nil {
		return nil, errors.Trace(err)
	}

	preparedInfo, err := prepareContainerInterfaceInfo(ctx, broker.api, containerMachineID, lxdLogger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	interfaces, err := finishNetworkConfig(preparedInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	net := container.BridgeNetworkConfig(0, interfaces)

	pNames, err := broker.writeProfiles(ctx, containerMachineID)
	if err != nil {
		err = fmt.Errorf("cannot write charm profile: %w", err)
		return nil, errors.WithType(err, environs.ErrAvailabilityZoneIndependent)
	}

	// The provisioner worker will provide all tools it knows about
	// (after applying explicitly specified constraints), which may
	// include tools for architectures other than the host's. We
	// must constrain to the host's architecture for LXD.
	archTools, err := matchHostArchTools(args.Tools)
	if err != nil {
		return nil, errors.Trace(err)
	}

	args.InstanceConfig.MachineContainerType = instance.LXD
	if err := args.InstanceConfig.SetTools(archTools); err != nil {
		return nil, errors.Trace(err)
	}

	cloudInitUserData, err := combinedCloudInitData(
		config.CloudInitUserData,
		config.ContainerInheritProperties,
		args.InstanceConfig.Base, lxdLogger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := instancecfg.PopulateInstanceConfig(
		args.InstanceConfig,
		config.ProviderType,
		config.SSLHostnameVerification,
		proxyConfigurationFromContainerCfg(config),
		config.EnableOSRefreshUpdate,
		config.EnableOSUpgrade,
		cloudInitUserData,
		append([]string{"default"}, pNames...),
	); err != nil {
		lxdLogger.Errorf(context.TODO(), "failed to populate machine config: %v", err)
		return nil, err
	}

	storageConfig := &container.StorageConfig{}
	inst, hardware, err := broker.manager.CreateContainer(
		ctx, args.InstanceConfig, args.Constraints, args.InstanceConfig.Base, net, storageConfig, args.StatusCallback,
	)
	if err != nil {
		return nil, err
	}

	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: hardware,
	}, nil
}

func (broker *lxdBroker) StopInstances(ctx envcontext.ProviderCallContext, ids ...instance.Id) error {
	// TODO: potentially parallelise.
	for _, id := range ids {
		lxdLogger.Infof(context.TODO(), "stopping lxd container for instance: %s", id)
		if err := broker.manager.DestroyContainer(id); err != nil {
			lxdLogger.Errorf(context.TODO(), "container did not stop: %v", err)
			return err
		}
		releaseContainerAddresses(ctx, broker.api, id, broker.manager.Namespace(), lxdLogger)
	}
	return nil
}

// AllInstances returns all containers.
func (broker *lxdBroker) AllInstances(ctx context.Context) (result []instances.Instance, err error) {
	return broker.manager.ListContainers()
}

// AllRunningInstances only returns running containers.
func (broker *lxdBroker) AllRunningInstances(ctx context.Context) (result []instances.Instance, err error) {
	return broker.manager.ListContainers()
}

// LXDProfileNames returns all the profiles for a container that the broker
// currently is aware of.
// LXDProfileNames implements environs.LXDProfiler.
func (broker *lxdBroker) LXDProfileNames(containerName string) ([]string, error) {
	nameRetriever, ok := broker.manager.(container.LXDProfileNameRetriever)
	if !ok {
		return make([]string, 0), nil
	}
	return nameRetriever.LXDProfileNames(containerName)
}

func (broker *lxdBroker) writeProfiles(ctx context.Context, machineID string) ([]string, error) {
	containerTag := names.NewMachineTag(machineID)
	profileInfo, err := broker.api.GetContainerProfileInfo(ctx, containerTag)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, profile := range profileInfo {
		if profile == nil {
			continue
		}
		if profile.Name == "" {
			return nil, errors.Errorf("request to write LXD profile for machine %s with no profile name", machineID)
		}
		err := broker.MaybeWriteLXDProfile(profile.Name, lxdprofile.Profile{
			Config:      profile.Config,
			Description: profile.Description,
			Devices:     profile.Devices,
		})
		if err != nil {
			return nil, err
		}
		names = append(names, profile.Name)
	}
	return names, nil
}

// MaybeWriteLXDProfile implements environs.LXDProfiler.
func (broker *lxdBroker) MaybeWriteLXDProfile(pName string, put lxdprofile.Profile) error {
	profileMgr, ok := broker.manager.(container.LXDProfileManager)
	if !ok {
		return nil
	}
	return profileMgr.MaybeWriteLXDProfile(pName, put)
}

// AssignLXDProfiles implements environs.LXDProfiler.
func (broker *lxdBroker) AssignLXDProfiles(instID string, profilesNames []string, profilePosts []lxdprofile.ProfilePost) ([]string, error) {
	profileMgr, ok := broker.manager.(container.LXDProfileManager)
	if !ok {
		return []string{}, nil
	}
	return profileMgr.AssignLXDProfiles(instID, profilesNames, profilePosts)
}
