// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/registry"
)

var logger = loggo.GetLogger("juju.apiserver.provisioner")

func init() {
	common.RegisterStandardFacade("Provisioner", 0, NewProvisionerAPI)

	// Version 1 has the same set of methods as 0, with the same
	// signatures, but its ProvisioningInfo returns additional
	// information. Clients may require version 1 so that they
	// receive this additional information; otherwise they are
	// compatible.
	common.RegisterStandardFacade("Provisioner", 1, NewProvisionerAPI)
}

// ProvisionerAPI provides access to the Provisioner API facade.
type ProvisionerAPI struct {
	*common.Remover
	*common.StatusSetter
	*common.StatusGetter
	*common.DeadEnsurer
	*common.PasswordChanger
	*common.LifeGetter
	*common.StateAddresser
	*common.APIAddresser
	*common.EnvironWatcher
	*common.EnvironMachinesWatcher
	*common.InstanceIdGetter
	*common.ToolsFinder
	*common.ToolsGetter

	st          *state.State
	resources   *common.Resources
	authorizer  common.Authorizer
	getAuthFunc common.GetAuthFunc
}

// NewProvisionerAPI creates a new server-side ProvisionerAPI facade.
func NewProvisionerAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*ProvisionerAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	getAuthFunc := func() (common.AuthFunc, error) {
		isEnvironManager := authorizer.AuthEnvironManager()
		isMachineAgent := authorizer.AuthMachineAgent()
		authEntityTag := authorizer.GetAuthTag()

		return func(tag names.Tag) bool {
			if isMachineAgent && tag == authEntityTag {
				// A machine agent can always access its own machine.
				return true
			}
			switch tag := tag.(type) {
			case names.MachineTag:
				parentId := state.ParentId(tag.Id())
				if parentId == "" {
					// All top-level machines are accessible by the
					// environment manager.
					return isEnvironManager
				}
				// All containers with the authenticated machine as a
				// parent are accessible by it.
				// TODO(dfc) sometimes authEntity tag is nil, which is fine because nil is
				// only equal to nil, but it suggests someone is passing an authorizer
				// with a nil tag.
				return isMachineAgent && names.NewMachineTag(parentId) == authEntityTag
			default:
				return false
			}
		}, nil
	}
	getAuthOwner := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	env, err := st.Environment()
	if err != nil {
		return nil, err
	}
	urlGetter := common.NewToolsURLGetter(env.UUID(), st)
	return &ProvisionerAPI{
		Remover:                common.NewRemover(st, false, getAuthFunc),
		StatusSetter:           common.NewStatusSetter(st, getAuthFunc),
		StatusGetter:           common.NewStatusGetter(st, getAuthFunc),
		DeadEnsurer:            common.NewDeadEnsurer(st, getAuthFunc),
		PasswordChanger:        common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:             common.NewLifeGetter(st, getAuthFunc),
		StateAddresser:         common.NewStateAddresser(st),
		APIAddresser:           common.NewAPIAddresser(st, resources),
		EnvironWatcher:         common.NewEnvironWatcher(st, resources, authorizer),
		EnvironMachinesWatcher: common.NewEnvironMachinesWatcher(st, resources, authorizer),
		InstanceIdGetter:       common.NewInstanceIdGetter(st, getAuthFunc),
		ToolsFinder:            common.NewToolsFinder(st, st, urlGetter),
		ToolsGetter:            common.NewToolsGetter(st, st, st, urlGetter, getAuthOwner),
		st:                     st,
		resources:              resources,
		authorizer:             authorizer,
		getAuthFunc:            getAuthFunc,
	}, nil
}

func (p *ProvisionerAPI) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (*state.Machine, error) {
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	entity, err := p.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	return entity.(*state.Machine), nil
}

func (p *ProvisionerAPI) watchOneMachineContainers(arg params.WatchContainer) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return nothing, common.ErrPerm
	}
	tag, err := names.ParseMachineTag(arg.MachineTag)
	if err != nil {
		return nothing, common.ErrPerm
	}
	if !canAccess(tag) {
		return nothing, common.ErrPerm
	}
	machine, err := p.st.Machine(tag.Id())
	if err != nil {
		return nothing, err
	}
	var watch state.StringsWatcher
	if arg.ContainerType != "" {
		watch = machine.WatchContainers(instance.ContainerType(arg.ContainerType))
	} else {
		watch = machine.WatchAllContainers()
	}
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: p.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// WatchContainers starts a StringsWatcher to watch containers deployed to
// any machine passed in args.
func (p *ProvisionerAPI) WatchContainers(args params.WatchContainers) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Params)),
	}
	for i, arg := range args.Params {
		watcherResult, err := p.watchOneMachineContainers(arg)
		result.Results[i] = watcherResult
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchAllContainers starts a StringsWatcher to watch all containers deployed to
// any machine passed in args.
func (p *ProvisionerAPI) WatchAllContainers(args params.WatchContainers) (params.StringsWatchResults, error) {
	return p.WatchContainers(args)
}

// SetSupportedContainers updates the list of containers supported by the machines passed in args.
func (p *ProvisionerAPI) SetSupportedContainers(args params.MachineContainersParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Params)),
	}

	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Params {
		tag, err := names.ParseMachineTag(arg.MachineTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := p.getMachine(canAccess, tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if len(arg.ContainerTypes) == 0 {
			err = machine.SupportsNoContainers()
		} else {
			err = machine.SetSupportedContainers(arg.ContainerTypes)
		}
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// ContainerManagerConfig returns information from the environment config that is
// needed for configuring the container manager.
func (p *ProvisionerAPI) ContainerManagerConfig(args params.ContainerManagerConfigParams) (params.ContainerManagerConfig, error) {
	var result params.ContainerManagerConfig
	config, err := p.st.EnvironConfig()
	if err != nil {
		return result, err
	}
	cfg := make(map[string]string)
	cfg[container.ConfigName] = container.DefaultNamespace

	switch args.Type {
	case instance.LXC:
		if useLxcClone, ok := config.LXCUseClone(); ok {
			cfg["use-clone"] = fmt.Sprint(useLxcClone)
		}
		if useLxcCloneAufs, ok := config.LXCUseCloneAUFS(); ok {
			cfg["use-aufs"] = fmt.Sprint(useLxcCloneAufs)
		}
		if lxcDefaultMTU, ok := config.LXCDefaultMTU(); ok {
			logger.Debugf("using default MTU %v for all LXC containers NICs", lxcDefaultMTU)
			cfg[container.ConfigLXCDefaultMTU] = fmt.Sprintf("%d", lxcDefaultMTU)
		}
	}

	if !environs.AddressAllocationEnabled() {
		// No need to even try checking the environ for support.
		logger.Debugf("address allocation feature flag not enabled")
		result.ManagerConfig = cfg
		return result, nil
	}

	// Create an environment to verify networking support.
	env, err := environs.New(config)
	if err != nil {
		return result, err
	}
	if netEnv, ok := environs.SupportsNetworking(env); ok {
		// Passing network.AnySubnet below should be interpreted by
		// the provider as "does ANY subnet support this".
		supported, err := netEnv.SupportsAddressAllocation(network.AnySubnet)
		if err == nil && supported {
			cfg[container.ConfigIPForwarding] = "true"
		} else if err != nil {
			// We log the error, but it's safe to ignore as it's not
			// critical.
			logger.Debugf("address allocation not supported (%v)", err)
		}
		// AWS requires NAT in place in order for hosted containers to
		// reach outside.
		if config.Type() == provider.EC2 {
			cfg[container.ConfigEnableNAT] = "true"
		}
	}

	result.ManagerConfig = cfg
	return result, nil
}

// ContainerConfig returns information from the environment config that is
// needed for container cloud-init.
func (p *ProvisionerAPI) ContainerConfig() (params.ContainerConfig, error) {
	result := params.ContainerConfig{}
	config, err := p.st.EnvironConfig()
	if err != nil {
		return result, err
	}

	result.UpdateBehavior = &params.UpdateBehavior{
		config.EnableOSRefreshUpdate(),
		config.EnableOSUpgrade(),
	}
	result.ProviderType = config.Type()
	result.AuthorizedKeys = config.AuthorizedKeys()
	result.SSLHostnameVerification = config.SSLHostnameVerification()
	result.Proxy = config.ProxySettings()
	result.AptProxy = config.AptProxySettings()
	result.PreferIPv6 = config.PreferIPv6()
	result.AllowLXCLoopMounts, _ = config.AllowLXCLoopMounts()

	return result, nil
}

// MachinesWithTransientErrors returns status data for machines with provisioning
// errors which are transient.
func (p *ProvisionerAPI) MachinesWithTransientErrors() (params.StatusResults, error) {
	var results params.StatusResults
	canAccessFunc, err := p.getAuthFunc()
	if err != nil {
		return results, err
	}
	// TODO (wallyworld) - add state.State API for more efficient machines query
	machines, err := p.st.AllMachines()
	if err != nil {
		return results, err
	}
	for _, machine := range machines {
		if !canAccessFunc(machine.Tag()) {
			continue
		}
		if _, provisionedErr := machine.InstanceId(); provisionedErr == nil {
			// Machine may have been provisioned but machiner hasn't set the
			// status to Started yet.
			continue
		}
		var result params.StatusResult
		statusInfo, err := machine.Status()
		if err != nil {
			continue
		}
		result.Status = params.Status(statusInfo.Status)
		result.Info = statusInfo.Message
		result.Data = statusInfo.Data
		if result.Status != params.StatusError {
			continue
		}
		// Transient errors are marked as such in the status data.
		if transient, ok := result.Data["transient"].(bool); !ok || !transient {
			continue
		}
		result.Id = machine.Id()
		result.Life = params.Life(machine.Life().String())
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// Series returns the deployed series for each given machine entity.
func (p *ProvisionerAPI) Series(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := p.getMachine(canAccess, tag)
		if err == nil {
			result.Results[i].Result = machine.Series()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ProvisioningInfo returns the provisioning information for each given machine entity.
func (p *ProvisionerAPI) ProvisioningInfo(args params.Entities) (params.ProvisioningInfoResults, error) {
	result := params.ProvisioningInfoResults{
		Results: make([]params.ProvisioningInfoResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := p.getMachine(canAccess, tag)
		if err == nil {
			result.Results[i].Result, err = p.getProvisioningInfo(machine)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (p *ProvisionerAPI) getProvisioningInfo(m *state.Machine) (*params.ProvisioningInfo, error) {
	cons, err := m.Constraints()
	if err != nil {
		return nil, err
	}
	volumes, err := p.machineVolumeParams(m)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(dimitern) Drop this once we only use spaces for
	// deployments.
	networks, err := m.RequestedNetworks()
	if err != nil {
		return nil, err
	}
	var jobs []multiwatcher.MachineJob
	for _, job := range m.Jobs() {
		jobs = append(jobs, job.ToParams())
	}
	tags, err := p.machineTags(m, jobs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetsToZones, err := p.machineSubnetsAndZones(m)
	if err != nil {
		return nil, errors.Annotate(err, "cannot match subnets to zones")
	}
	return &params.ProvisioningInfo{
		Constraints:    cons,
		Series:         m.Series(),
		Placement:      m.Placement(),
		Networks:       networks,
		Jobs:           jobs,
		Volumes:        volumes,
		Tags:           tags,
		SubnetsToZones: subnetsToZones,
	}, nil
}

// DistributionGroup returns, for each given machine entity,
// a slice of instance.Ids that belong to the same distribution
// group as that machine. This information may be used to
// distribute instances for high availability.
func (p *ProvisionerAPI) DistributionGroup(args params.Entities) (params.DistributionGroupResults, error) {
	result := params.DistributionGroupResults{
		Results: make([]params.DistributionGroupResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := p.getMachine(canAccess, tag)
		if err == nil {
			// If the machine is an environment manager, return
			// environment manager instances. Otherwise, return
			// instances with services in common with the machine
			// being provisioned.
			if machine.IsManager() {
				result.Results[i].Result, err = environManagerInstances(p.st)
			} else {
				result.Results[i].Result, err = commonServiceInstances(p.st, machine)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// environManagerInstances returns all environ manager instances.
func environManagerInstances(st *state.State) ([]instance.Id, error) {
	info, err := st.StateServerInfo()
	if err != nil {
		return nil, err
	}
	instances := make([]instance.Id, 0, len(info.MachineIds))
	for _, id := range info.MachineIds {
		machine, err := st.Machine(id)
		if err != nil {
			return nil, err
		}
		instanceId, err := machine.InstanceId()
		if err == nil {
			instances = append(instances, instanceId)
		} else if !errors.IsNotProvisioned(err) {
			return nil, err
		}
	}
	return instances, nil
}

// commonServiceInstances returns instances with
// services in common with the specified machine.
func commonServiceInstances(st *state.State, m *state.Machine) ([]instance.Id, error) {
	units, err := m.Units()
	if err != nil {
		return nil, err
	}
	instanceIdSet := make(set.Strings)
	for _, unit := range units {
		if !unit.IsPrincipal() {
			continue
		}
		instanceIds, err := state.ServiceInstances(st, unit.ServiceName())
		if err != nil {
			return nil, err
		}
		for _, instanceId := range instanceIds {
			instanceIdSet.Add(string(instanceId))
		}
	}
	instanceIds := make([]instance.Id, instanceIdSet.Size())
	// Sort values to simplify testing.
	for i, instanceId := range instanceIdSet.SortedValues() {
		instanceIds[i] = instance.Id(instanceId)
	}
	return instanceIds, nil
}

// Constraints returns the constraints for each given machine entity.
func (p *ProvisionerAPI) Constraints(args params.Entities) (params.ConstraintsResults, error) {
	result := params.ConstraintsResults{
		Results: make([]params.ConstraintsResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := p.getMachine(canAccess, tag)
		if err == nil {
			var cons constraints.Value
			cons, err = machine.Constraints()
			if err == nil {
				result.Results[i].Constraints = cons
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// machineVolumeParams retrieves VolumeParams for the volumes that should be
// provisioned with, and attached to, the machine. The client should ignore
// parameters that it does not know how to handle.
func (p *ProvisionerAPI) machineVolumeParams(m *state.Machine) ([]params.VolumeParams, error) {
	volumeAttachments, err := m.VolumeAttachments()
	if err != nil {
		return nil, err
	}
	if len(volumeAttachments) == 0 {
		return nil, nil
	}
	envConfig, err := p.st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	poolManager := poolmanager.New(state.NewStateSettings(p.st))
	allVolumeParams := make([]params.VolumeParams, 0, len(volumeAttachments))
	for _, volumeAttachment := range volumeAttachments {
		volumeTag := volumeAttachment.Volume()
		volume, err := p.st.Volume(volumeTag)
		if err != nil {
			return nil, errors.Annotatef(err, "getting volume %q", volumeTag.Id())
		}
		storageInstance, err := common.MaybeAssignedStorageInstance(
			volume.StorageInstance, p.st.StorageInstance,
		)
		if err != nil {
			return nil, errors.Annotatef(err, "getting volume %q storage instance", volumeTag.Id())
		}
		volumeParams, err := common.VolumeParams(volume, storageInstance, envConfig, poolManager)
		if err != nil {
			return nil, errors.Annotatef(err, "getting volume %q parameters", volumeTag.Id())
		}
		provider, err := registry.StorageProvider(storage.ProviderType(volumeParams.Provider))
		if err != nil {
			return nil, errors.Annotate(err, "getting storage provider")
		}
		if provider.Dynamic() {
			// Leave dynamic storage to the storage provisioner.
			continue
		}
		volumeAttachmentParams, ok := volumeAttachment.Params()
		if !ok {
			// Attachment is already provisioned; this is an insane
			// state, so we should not proceed with the volume.
			return nil, errors.Errorf(
				"volume %s already attached to machine %s",
				volumeTag.Id(), m.Id(),
			)
		}
		// Not provisioned yet, so ask the cloud provisioner do it.
		volumeParams.Attachment = &params.VolumeAttachmentParams{
			volumeTag.String(),
			m.Tag().String(),
			"", // we're creating the volume, so it has no volume ID.
			"", // we're creating the machine, so it has no instance ID.
			volumeParams.Provider,
			volumeAttachmentParams.ReadOnly,
		}
		allVolumeParams = append(allVolumeParams, volumeParams)
	}
	return allVolumeParams, nil
}

// storageConfig returns the provider type and config attributes for the
// specified poolName. If no such pool exists, we check to see if poolName is
// actually a provider type, in which case config will be empty.
func storageConfig(st *state.State, poolName string) (storage.ProviderType, map[string]interface{}, error) {
	pm := poolmanager.New(state.NewStateSettings(st))
	p, err := pm.Get(poolName)
	// If not a storage pool, then maybe a provider type.
	if errors.IsNotFound(err) {
		providerType := storage.ProviderType(poolName)
		if _, err1 := registry.StorageProvider(providerType); err1 != nil {
			return "", nil, errors.Trace(err)
		}
		return providerType, nil, nil
	}
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	return p.Provider(), p.Attrs(), nil
}

// volumeAttachmentsToState converts a slice of storage.VolumeAttachment to a
// mapping of volume names to state.VolumeAttachmentInfo.
func volumeAttachmentsToState(in []params.VolumeAttachment) (map[names.VolumeTag]state.VolumeAttachmentInfo, error) {
	m := make(map[names.VolumeTag]state.VolumeAttachmentInfo)
	for _, v := range in {
		if v.VolumeTag == "" {
			return nil, errors.New("Tag is empty")
		}
		volumeTag, err := names.ParseVolumeTag(v.VolumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		m[volumeTag] = state.VolumeAttachmentInfo{
			v.Info.DeviceName,
			v.Info.BusAddress,
			v.Info.ReadOnly,
		}
	}
	return m, nil
}

func networkParamsToStateParams(networks []params.Network, ifaces []params.NetworkInterface) (
	[]state.NetworkInfo, []state.NetworkInterfaceInfo, error,
) {
	stateNetworks := make([]state.NetworkInfo, len(networks))
	for i, net := range networks {
		tag, err := names.ParseNetworkTag(net.Tag)
		if err != nil {
			return nil, nil, err
		}
		stateNetworks[i] = state.NetworkInfo{
			Name:       tag.Id(),
			ProviderId: network.Id(net.ProviderId),
			CIDR:       net.CIDR,
			VLANTag:    net.VLANTag,
		}
	}
	stateInterfaces := make([]state.NetworkInterfaceInfo, len(ifaces))
	for i, iface := range ifaces {
		tag, err := names.ParseNetworkTag(iface.NetworkTag)
		if err != nil {
			return nil, nil, err
		}
		stateInterfaces[i] = state.NetworkInterfaceInfo{
			MACAddress:    iface.MACAddress,
			NetworkName:   tag.Id(),
			InterfaceName: iface.InterfaceName,
			IsVirtual:     iface.IsVirtual,
			Disabled:      iface.Disabled,
		}
	}
	return stateNetworks, stateInterfaces, nil
}

// RequestedNetworks returns the requested networks for each given
// machine entity. Each entry in both lists is returned with its
// provider specific id.
func (p *ProvisionerAPI) RequestedNetworks(args params.Entities) (params.RequestedNetworksResults, error) {
	result := params.RequestedNetworksResults{
		Results: make([]params.RequestedNetworkResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := p.getMachine(canAccess, tag)
		if err == nil {
			var networks []string
			networks, err = machine.RequestedNetworks()
			if err == nil {
				// TODO(dimitern) For now, since network names and
				// provider ids are the same, we return what we got
				// from state. In the future, when networks can be
				// added before provisioning, we should convert both
				// slices from juju network names to provider-specific
				// ids before returning them.
				result.Results[i].Networks = networks
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetProvisioned sets the provider specific instance id, nonce and
// metadata for each given machine. Once set, the instance id cannot
// be changed.
//
// TODO(dimitern) This is not used anymore (as of 1.19.0) and is
// retained only for backwards-compatibility. It should be removed as
// deprecated. SetInstanceInfo is used instead.
func (p *ProvisionerAPI) SetProvisioned(args params.SetProvisioned) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Machines)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Machines {
		tag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := p.getMachine(canAccess, tag)
		if err == nil {
			err = machine.SetProvisioned(arg.InstanceId, arg.Nonce, arg.Characteristics)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetInstanceInfo sets the provider specific machine id, nonce,
// metadata and network info for each given machine. Once set, the
// instance id cannot be changed.
func (p *ProvisionerAPI) SetInstanceInfo(args params.InstancesInfo) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Machines)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		return result, err
	}
	setInstanceInfo := func(arg params.InstanceInfo) error {
		tag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			return common.ErrPerm
		}
		machine, err := p.getMachine(canAccess, tag)
		if err != nil {
			return err
		}
		networks, interfaces, err := networkParamsToStateParams(arg.Networks, arg.Interfaces)
		if err != nil {
			return err
		}
		volumes, err := common.VolumesToState(arg.Volumes)
		if err != nil {
			return err
		}
		volumeAttachments, err := common.VolumeAttachmentInfosToState(arg.VolumeAttachments)
		if err != nil {
			return err
		}
		if err = machine.SetInstanceInfo(
			arg.InstanceId, arg.Nonce, arg.Characteristics,
			networks, interfaces, volumes, volumeAttachments); err != nil {
			return errors.Annotatef(
				err,
				"cannot record provisioning info for %q",
				arg.InstanceId,
			)
		}
		return nil
	}
	for i, arg := range args.Machines {
		err := setInstanceInfo(arg)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchMachineErrorRetry returns a NotifyWatcher that notifies when
// the provisioner should retry provisioning machines with transient errors.
func (p *ProvisionerAPI) WatchMachineErrorRetry() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	if !p.authorizer.AuthEnvironManager() {
		return result, common.ErrPerm
	}
	watch := newWatchMachineErrorRetry()
	// Consume any initial event and forward it to the result.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = p.resources.Register(watch)
	} else {
		return result, watcher.EnsureErr(watch)
	}
	return result, nil
}

// ReleaseContainerAddresses finds addresses allocated to a container
// and marks them as Dead, to be released and removed. It accepts
// container tags as arguments. If address allocation feature flag is
// not enabled, it will return a NotSupported error.
func (p *ProvisionerAPI) ReleaseContainerAddresses(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	if !environs.AddressAllocationEnabled() {
		return result, errors.NotSupportedf("address allocation")
	}

	canAccess, err := p.getAuthFunc()
	if err != nil {
		logger.Errorf("failed to get an authorisation function: %v", err)
		return result, errors.Trace(err)
	}
	// Loop over the passed container tags.
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			logger.Warningf("failed to parse machine tag %q: %v", entity.Tag, err)
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		// The auth function (canAccess) checks that the machine is a
		// top level machine (we filter those out next) or that the
		// machine has the host as a parent.
		container, err := p.getMachine(canAccess, tag)
		if err != nil {
			logger.Warningf("failed to get machine %q: %v", tag, err)
			result.Results[i].Error = common.ServerError(err)
			continue
		} else if !container.IsContainer() {
			err = errors.Errorf("cannot mark addresses for removal for %q: not a container", tag)
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		id := container.Id()
		addresses, err := p.st.AllocatedIPAddresses(id)
		if err != nil {
			logger.Warningf("failed to get Id for container %q: %v", tag, err)
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		deadErrors := []error{}
		logger.Debugf("for container %q found addresses %v", tag, addresses)
		for _, addr := range addresses {
			err = addr.EnsureDead()
			if err != nil {
				deadErrors = append(deadErrors, err)
				continue
			}
		}
		if len(deadErrors) != 0 {
			err = errors.Errorf("failed to mark all addresses for removal for %q: %v", tag, deadErrors)
			result.Results[i].Error = common.ServerError(err)
		}
	}

	return result, nil
}

// PrepareContainerInterfaceInfo allocates an address and returns
// information to configure networking for a container. It accepts
// container tags as arguments. If the address allocation feature flag
// is not enabled, it returns a NotSupported error.
func (p *ProvisionerAPI) PrepareContainerInterfaceInfo(args params.Entities) (
	params.MachineNetworkConfigResults, error) {
	return p.prepareOrGetContainerInterfaceInfo(args, true)
}

// GetContainerInterfaceInfo returns information to configure networking
// for a container. It accepts container tags as arguments. If the address
// allocation feature flag is not enabled, it returns a NotSupported error.
func (p *ProvisionerAPI) GetContainerInterfaceInfo(args params.Entities) (
	params.MachineNetworkConfigResults, error) {
	return p.prepareOrGetContainerInterfaceInfo(args, false)
}

// MACAddressTemplate is used to generate a unique MAC address for a
// container. Every '%x' is replaced by a random hexadecimal digit,
// while the rest is kept as-is.
const MACAddressTemplate = "00:16:3e:%02x:%02x:%02x"

// generateMACAddress creates a random MAC address within the space defined by
// MACAddressTemplate above.
func generateMACAddress() string {
	digits := make([]interface{}, 3)
	for i := range digits {
		digits[i] = rand.Intn(256)
	}
	return fmt.Sprintf(MACAddressTemplate, digits...)
}

// prepareOrGetContainerInterfaceInfo optionally allocates an address and returns information
// for configuring networking on a container. It accepts container tags as arguments.
func (p *ProvisionerAPI) prepareOrGetContainerInterfaceInfo(
	args params.Entities, provisionContainer bool) (
	params.MachineNetworkConfigResults, error) {
	result := params.MachineNetworkConfigResults{
		Results: make([]params.MachineNetworkConfigResult, len(args.Entities)),
	}

	if !environs.AddressAllocationEnabled() {
		return result, errors.NotSupportedf("address allocation")
	}

	// Some preparations first.
	environ, host, canAccess, err := p.prepareContainerAccessEnvironment()
	if err != nil {
		return result, errors.Trace(err)
	}
	instId, err := host.InstanceId()
	if err != nil && errors.IsNotProvisioned(err) {
		// If the host machine is not provisioned yet, we have nothing
		// to do. NotProvisionedf will append " not provisioned" to
		// the message.
		err = errors.NotProvisionedf("cannot allocate addresses: host machine %q", host)
		return result, err
	}
	subnet, subnetInfo, interfaceInfo, err := p.prepareAllocationNetwork(environ, host, instId)
	if err != nil {
		return result, errors.Annotate(err, "cannot allocate addresses")
	}

	// Loop over the passed container tags.
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		// The auth function (canAccess) checks that the machine is a
		// top level machine (we filter those out next) or that the
		// machine has the host as a parent.
		container, err := p.getMachine(canAccess, tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		} else if !container.IsContainer() {
			err = errors.Errorf("cannot allocate address for %q: not a container", tag)
			result.Results[i].Error = common.ServerError(err)
			continue
		} else if ciid, cerr := container.InstanceId(); provisionContainer == true && cerr == nil {
			// Since we want to configure and create NICs on the
			// container before it starts, it must also be not
			// provisioned yet.
			err = errors.Errorf("container %q already provisioned as %q", container, ciid)
			result.Results[i].Error = common.ServerError(err)
			continue
		} else if cerr != nil && !errors.IsNotProvisioned(cerr) {
			// Any other error needs to be reported.
			result.Results[i].Error = common.ServerError(cerr)
			continue
		}

		var macAddress string
		var address *state.IPAddress
		if provisionContainer {
			// Allocate and set address.
			macAddress = generateMACAddress()
			address, err = p.allocateAddress(environ, subnet, host, container, instId, macAddress)
			if err != nil {
				err = errors.Annotatef(err, "failed to allocate an address for %q", container)
				result.Results[i].Error = common.ServerError(err)
				continue
			}
		} else {
			id := container.Id()
			addresses, err := p.st.AllocatedIPAddresses(id)
			if err != nil {
				logger.Warningf("failed to get Id for container %q: %v", tag, err)
				result.Results[i].Error = common.ServerError(err)
				continue
			}
			// TODO(dooferlad): if we get more than 1 address back, we ignore everything after
			// the first. The calling function expects exactly one result though,
			// so we don't appear to have a way of allocating >1 address to a
			// container...
			if len(addresses) != 1 {
				logger.Warningf("got %d addresses for container %q - expected 1: %v", len(addresses), tag, err)
				result.Results[i].Error = common.ServerError(err)
				continue
			}
			address = addresses[0]
			macAddress = address.MACAddress()
		}
		// Store it on the machine, construct and set an interface result.
		dnsServers := make([]string, len(interfaceInfo.DNSServers))
		for i, dns := range interfaceInfo.DNSServers {
			dnsServers[i] = dns.Value
		}

		if macAddress == "" {
			macAddress = interfaceInfo.MACAddress
		}
		// TODO(dimitern): Support allocating one address per NIC on
		// the host, effectively creating the same number of NICs in
		// the container.
		result.Results[i] = params.MachineNetworkConfigResult{
			Config: []params.NetworkConfig{{
				DeviceIndex:      interfaceInfo.DeviceIndex,
				MACAddress:       macAddress,
				CIDR:             subnetInfo.CIDR,
				NetworkName:      interfaceInfo.NetworkName,
				ProviderId:       string(interfaceInfo.ProviderId),
				ProviderSubnetId: string(subnetInfo.ProviderId),
				VLANTag:          interfaceInfo.VLANTag,
				InterfaceName:    interfaceInfo.InterfaceName,
				Disabled:         interfaceInfo.Disabled,
				NoAutoStart:      interfaceInfo.NoAutoStart,
				DNSServers:       dnsServers,
				ConfigType:       string(network.ConfigStatic),
				Address:          address.Value(),
				// container's gateway is the host's primary NIC's IP.
				GatewayAddress: interfaceInfo.Address.Value,
				ExtraConfig:    interfaceInfo.ExtraConfig,
			}},
		}
	}
	return result, nil
}

// prepareContainerAccessEnvironment retrieves the environment, host machine, and access
// for working with containers.
func (p *ProvisionerAPI) prepareContainerAccessEnvironment() (environs.NetworkingEnviron, *state.Machine, common.AuthFunc, error) {
	cfg, err := p.st.EnvironConfig()
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "failed to get environment config")
	}
	environ, err := environs.New(cfg)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "failed to construct an environment from config")
	}
	netEnviron, supported := environs.SupportsNetworking(environ)
	if !supported {
		// " not supported" will be appended to the message below.
		return nil, nil, nil, errors.NotSupportedf("environment %q networking", cfg.Name())
	}

	canAccess, err := p.getAuthFunc()
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "cannot authenticate request")
	}
	hostAuthTag := p.authorizer.GetAuthTag()
	if hostAuthTag == nil {
		return nil, nil, nil, errors.Errorf("authenticated entity tag is nil")
	}
	hostTag, err := names.ParseMachineTag(hostAuthTag.String())
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	host, err := p.getMachine(canAccess, hostTag)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	return netEnviron, host, canAccess, nil
}

// prepareAllocationNetwork retrieves the subnet, its info, and the interface info
// for the allocations.
func (p *ProvisionerAPI) prepareAllocationNetwork(
	environ environs.NetworkingEnviron,
	host *state.Machine,
	instId instance.Id,
) (
	*state.Subnet,
	network.SubnetInfo,
	network.InterfaceInfo,
	error,
) {
	var subnetInfo network.SubnetInfo
	var interfaceInfo network.InterfaceInfo

	interfaces, err := environ.NetworkInterfaces(instId)
	if err != nil {
		return nil, subnetInfo, interfaceInfo, errors.Trace(err)
	} else if len(interfaces) == 0 {
		return nil, subnetInfo, interfaceInfo, errors.Errorf("no interfaces available")
	}
	logger.Tracef("interfaces for instance %q: %v", instId, interfaces)

	subnetSet := make(set.Strings)
	subnetIds := []network.Id{}
	subnetIdToInterface := make(map[network.Id]network.InterfaceInfo)
	for _, iface := range interfaces {
		if iface.ProviderSubnetId == "" {
			logger.Debugf("no subnet associated with interface %#v (skipping)", iface)
			continue
		} else if iface.Disabled {
			logger.Debugf("interface %#v disabled (skipping)", iface)
			continue
		}
		if !subnetSet.Contains(string(iface.ProviderSubnetId)) {
			subnetIds = append(subnetIds, iface.ProviderSubnetId)
			subnetSet.Add(string(iface.ProviderSubnetId))

			// This means that multiple interfaces on the same subnet will
			// only appear once.
			subnetIdToInterface[iface.ProviderSubnetId] = iface
		}
	}
	subnets, err := environ.Subnets(instId, subnetIds)
	if err != nil {
		return nil, subnetInfo, interfaceInfo, errors.Trace(err)
	} else if len(subnets) == 0 {
		return nil, subnetInfo, interfaceInfo, errors.Errorf("no subnets available")
	}
	logger.Tracef("subnets for instance %q: %v", instId, subnets)

	// TODO(mfoord): we need a better strategy for picking a subnet to
	// allocate an address on. (dimitern): Right now we just pick the
	// first subnet with allocatable range set. Instead, we should
	// allocate an address per interface, assuming each interface is
	// on a subnet with allocatable range set, and skipping those
	// which do not have a range set.
	var success bool
	for _, sub := range subnets {
		logger.Tracef("trying to allocate a static IP on subnet %q", sub.ProviderId)
		if sub.AllocatableIPHigh == nil {
			logger.Tracef("ignoring subnet %q - no allocatable range set", sub.ProviderId)
			// this subnet has no allocatable IPs
			continue
		}
		ok, err := environ.SupportsAddressAllocation(sub.ProviderId)
		if err == nil && ok {
			subnetInfo = sub
			interfaceInfo = subnetIdToInterface[sub.ProviderId]
			success = true
			break
		}
		logger.Tracef(
			"subnet %q supports address allocation: %v (error: %v)",
			sub.ProviderId, ok, err,
		)
	}
	if !success {
		// " not supported" will be appended to the message below.
		return nil, subnetInfo, interfaceInfo, errors.NotSupportedf(
			"address allocation on any available subnets is",
		)
	}
	subnet, err := p.createOrFetchStateSubnet(subnetInfo)

	return subnet, subnetInfo, interfaceInfo, nil
}

// These are defined like this to allow mocking in tests.
var (
	allocateAddrTo = func(a *state.IPAddress, m *state.Machine, macAddress string) error {
		// TODO(mfoord): populate proper interface ID (in state).
		return a.AllocateTo(m.Id(), "", macAddress)
	}
	setAddrsTo = func(a *state.IPAddress, m *state.Machine) error {
		return m.SetProviderAddresses(a.Address())
	}
	setAddrState = func(a *state.IPAddress, st state.AddressState) error {
		return a.SetState(st)
	}
)

// allocateAddress tries to pick an address out of the given subnet and
// allocates it to the container.
func (p *ProvisionerAPI) allocateAddress(
	environ environs.NetworkingEnviron,
	subnet *state.Subnet,
	host, container *state.Machine,
	instId instance.Id,
	macAddress string,
) (*state.IPAddress, error) {

	subnetId := network.Id(subnet.ProviderId())
	name := names.NewMachineTag(container.Id()).String()
	for {
		addr, err := subnet.PickNewAddress()
		if err != nil {
			return nil, err
		}
		logger.Tracef("picked new address %q on subnet %q", addr.String(), subnetId)
		// Attempt to allocate with environ.
		err = environ.AllocateAddress(instId, subnetId, addr.Address(), macAddress, name)
		if err != nil {
			logger.Warningf(
				"allocating address %q on instance %q and subnet %q failed: %v (retrying)",
				addr.String(), instId, subnetId, err,
			)
			// It's as good as unavailable for us, so mark it as
			// such.
			err = setAddrState(addr, state.AddressStateUnavailable)
			if err != nil {
				logger.Warningf(
					"cannot set address %q to %q: %v (ignoring and retrying)",
					addr.String(), state.AddressStateUnavailable, err,
				)
				continue
			}
			logger.Tracef(
				"setting address %q to %q and retrying",
				addr.String(), state.AddressStateUnavailable,
			)
			continue
		}
		logger.Infof(
			"allocated address %q on instance %q and subnet %q",
			addr.String(), instId, subnetId,
		)
		err = p.setAllocatedOrRelease(addr, environ, instId, container, subnetId, macAddress)
		if err != nil {
			// Something went wrong - retry.
			continue
		}
		return addr, nil
	}
}

// setAllocatedOrRelease tries to associate the newly allocated
// address addr with the container. On failure it makes the best
// effort to cleanup and release addr, logging issues along the way.
func (p *ProvisionerAPI) setAllocatedOrRelease(
	addr *state.IPAddress,
	environ environs.NetworkingEnviron,
	instId instance.Id,
	container *state.Machine,
	subnetId network.Id,
	macAddress string,
) (err error) {
	defer func() {
		if errors.Cause(err) == nil {
			// Success!
			return
		}
		logger.Warningf(
			"failed to mark address %q as %q to container %q: %v (releasing and retrying)",
			addr.String(), state.AddressStateAllocated, container, err,
		)
		// It's as good as unavailable for us, so mark it as
		// such.
		err = setAddrState(addr, state.AddressStateUnavailable)
		if err != nil {
			logger.Warningf(
				"cannot set address %q to %q: %v (ignoring and releasing)",
				addr.String(), state.AddressStateUnavailable, err,
			)
		}
		err = environ.ReleaseAddress(instId, subnetId, addr.Address(), addr.MACAddress())
		if err == nil {
			logger.Infof("address %q released; trying to allocate new", addr.String())
			return
		}
		logger.Warningf(
			"failed to release address %q on instance %q and subnet %q: %v (ignoring and retrying)",
			addr.String(), instId, subnetId, err,
		)
	}()

	// Any errors returned below will trigger the release/cleanup
	// steps above.
	if err = allocateAddrTo(addr, container, macAddress); err != nil {
		return errors.Trace(err)
	}
	if err = setAddrsTo(addr, container); err != nil {
		return errors.Trace(err)
	}

	logger.Infof("assigned address %q to container %q", addr.String(), container)
	return nil
}

func (p *ProvisionerAPI) createOrFetchStateSubnet(subnetInfo network.SubnetInfo) (*state.Subnet, error) {
	stateSubnetInfo := state.SubnetInfo{
		ProviderId:        string(subnetInfo.ProviderId),
		CIDR:              subnetInfo.CIDR,
		VLANTag:           subnetInfo.VLANTag,
		AllocatableIPHigh: subnetInfo.AllocatableIPHigh.String(),
		AllocatableIPLow:  subnetInfo.AllocatableIPLow.String(),
	}
	subnet, err := p.st.AddSubnet(stateSubnetInfo)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			subnet, err = p.st.Subnet(subnetInfo.CIDR)
		}
		if err != nil {
			return subnet, errors.Trace(err)
		}
	}
	return subnet, nil
}

// machineTags returns machine-specific tags to set on the instance.
func (p *ProvisionerAPI) machineTags(m *state.Machine, jobs []multiwatcher.MachineJob) (map[string]string, error) {
	// Names of all units deployed to the machine.
	//
	// TODO(axw) 2015-06-02 #1461358
	// We need a worker that periodically updates
	// instance tags with current deployment info.
	units, err := m.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitNames := make([]string, 0, len(units))
	for _, unit := range units {
		if !unit.IsPrincipal() {
			continue
		}
		unitNames = append(unitNames, unit.Name())
	}
	sort.Strings(unitNames)

	cfg, err := p.st.EnvironConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineTags := instancecfg.InstanceTags(cfg, jobs)
	if len(unitNames) > 0 {
		machineTags[tags.JujuUnitsDeployed] = strings.Join(unitNames, " ")
	}
	return machineTags, nil
}

// machineSubnetsAndZones returns a map of subnet provider-specific id
// to list of availability zone names for that subnet. The result can
// be empty if there are no spaces constraints specified for the
// machine, or there's an error fetching them.
func (p *ProvisionerAPI) machineSubnetsAndZones(m *state.Machine) (map[string][]string, error) {
	mcons, err := m.Constraints()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get machine constraints")
	}
	includeSpaces := mcons.IncludeSpaces()
	if len(includeSpaces) < 1 {
		// Nothing to do.
		return nil, nil
	}
	// TODO(dimitern): For the network model MVP we only use the first
	// included space and ignore the rest.
	spaceName := includeSpaces[0]
	if len(includeSpaces) > 1 {
		logger.Debugf(
			"using space %q from constraints for machine %q (ignoring remaining: %v)",
			spaceName, m.Id(), includeSpaces[1:],
		)
	}
	space, err := p.st.Space(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets, err := space.Subnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetsToZones := make(map[string][]string, len(subnets))
	for _, subnet := range subnets {
		warningPrefix := fmt.Sprintf(
			"not using subnet %q in space %q for machine %q provisioning: ",
			subnet.CIDR(), spaceName, m.Id(),
		)
		// TODO(dimitern): state.Subnet.ProviderId needs to be of type
		// network.Id.
		providerId := subnet.ProviderId()
		if providerId == "" {
			logger.Warningf(warningPrefix + "no ProviderId set")
			continue
		}
		// TODO(dimitern): Once state.Subnet supports multiple zones,
		// use all of them below.
		zone := subnet.AvailabilityZone()
		if zone == "" {
			logger.Warningf(warningPrefix + "no availability zone(s) set")
			continue
		}
		subnetsToZones[providerId] = []string{zone}
	}
	return subnetsToZones, nil
}
