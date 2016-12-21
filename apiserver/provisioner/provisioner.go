// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

var logger = loggo.GetLogger("juju.apiserver.provisioner")

func init() {
	common.RegisterStandardFacade("Provisioner", 3, NewProvisionerAPI)
}

// ProvisionerAPI provides access to the Provisioner API facade.
type ProvisionerAPI struct {
	*common.ControllerConfigAPI
	*common.Remover
	*common.StatusSetter
	*common.StatusGetter
	*common.DeadEnsurer
	*common.PasswordChanger
	*common.LifeGetter
	*common.StateAddresser
	*common.APIAddresser
	*common.ModelWatcher
	*common.ModelMachinesWatcher
	*common.InstanceIdGetter
	*common.ToolsFinder
	*common.ToolsGetter

	st                      *state.State
	resources               facade.Resources
	authorizer              facade.Authorizer
	storageProviderRegistry storage.ProviderRegistry
	storagePoolManager      poolmanager.PoolManager
	configGetter            environs.EnvironConfigGetter
	getAuthFunc             common.GetAuthFunc
}

// NewProvisionerAPI creates a new server-side ProvisionerAPI facade.
func NewProvisionerAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*ProvisionerAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}
	getAuthFunc := func() (common.AuthFunc, error) {
		isModelManager := authorizer.AuthModelManager()
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
					return isModelManager
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
	model, err := st.Model()
	if err != nil {
		return nil, err
	}
	configGetter := stateenvirons.EnvironConfigGetter{st}
	env, err := environs.GetEnviron(configGetter, environs.New)
	if err != nil {
		return nil, err
	}
	urlGetter := common.NewToolsURLGetter(model.UUID(), st)
	storageProviderRegistry := stateenvirons.NewStorageProviderRegistry(env)
	return &ProvisionerAPI{
		Remover:                 common.NewRemover(st, false, getAuthFunc),
		StatusSetter:            common.NewStatusSetter(st, getAuthFunc),
		StatusGetter:            common.NewStatusGetter(st, getAuthFunc),
		DeadEnsurer:             common.NewDeadEnsurer(st, getAuthFunc),
		PasswordChanger:         common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:              common.NewLifeGetter(st, getAuthFunc),
		StateAddresser:          common.NewStateAddresser(st),
		APIAddresser:            common.NewAPIAddresser(st, resources),
		ModelWatcher:            common.NewModelWatcher(st, resources, authorizer),
		ModelMachinesWatcher:    common.NewModelMachinesWatcher(st, resources, authorizer),
		ControllerConfigAPI:     common.NewControllerConfig(st),
		InstanceIdGetter:        common.NewInstanceIdGetter(st, getAuthFunc),
		ToolsFinder:             common.NewToolsFinder(configGetter, st, urlGetter),
		ToolsGetter:             common.NewToolsGetter(st, configGetter, st, urlGetter, getAuthOwner),
		st:                      st,
		resources:               resources,
		authorizer:              authorizer,
		configGetter:            configGetter,
		storageProviderRegistry: storageProviderRegistry,
		storagePoolManager:      poolmanager.New(state.NewStateSettings(st), storageProviderRegistry),
		getAuthFunc:             getAuthFunc,
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
	cfg := make(map[string]string)
	cfg[container.ConfigModelUUID] = p.st.ModelUUID()

	switch args.Type {
	case instance.LXD:
		// TODO(jam): DefaultMTU needs to be handled here
		// TODO(jam): Do we want to handle ImageStream here, or do we
		// hide it from them? (all cached images must come from the
		// same image stream?)
	}

	result.ManagerConfig = cfg
	return result, nil
}

// ContainerConfig returns information from the environment config that is
// needed for container cloud-init.
func (p *ProvisionerAPI) ContainerConfig() (params.ContainerConfig, error) {
	result := params.ContainerConfig{}
	config, err := p.st.ModelConfig()
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
	result.AptMirror = config.AptMirror()

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
		result.Status = statusInfo.Status.String()
		result.Info = statusInfo.Message
		result.Data = statusInfo.Data
		if statusInfo.Status != status.Error {
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
	info, err := st.ControllerInfo()
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
		instanceIds, err := state.ServiceInstances(st, unit.ApplicationName())
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
		volumes, err := storagecommon.VolumesToState(arg.Volumes)
		if err != nil {
			return err
		}
		volumeAttachments, err := storagecommon.VolumeAttachmentInfosToState(arg.VolumeAttachments)
		if err != nil {
			return err
		}

		devicesArgs, devicesAddrs := networkingcommon.NetworkConfigsToStateArgs(arg.NetworkConfig)

		err = machine.SetInstanceInfo(
			arg.InstanceId, arg.Nonce, arg.Characteristics,
			devicesArgs, devicesAddrs,
			volumes, volumeAttachments,
		)
		if err != nil {
			return errors.Annotatef(err, "cannot record provisioning info for %q", arg.InstanceId)
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
	if !p.authorizer.AuthModelManager() {
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

// ReleaseContainerAddresses finds addresses allocated to a container and marks
// them as Dead, to be released and removed. It accepts container tags as
// arguments.
func (p *ProvisionerAPI) ReleaseContainerAddresses(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
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

		// TODO(dimitern): Release those via the provider once we have
		// Environ.ReleaseContainerAddresses. See LP bug http://pad.lv/1585878
		err = container.RemoveAllAddresses()
		if err != nil {
			logger.Warningf("failed to remove container %q addresses: %v", tag, err)
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}

	return result, nil
}

// PrepareContainerInterfaceInfo allocates an address and returns information to
// configure networking for a container. It accepts container tags as arguments.
func (p *ProvisionerAPI) PrepareContainerInterfaceInfo(args params.Entities) (
	params.MachineNetworkConfigResults,
	error,
) {
	return p.prepareOrGetContainerInterfaceInfo(args, false)
}

// GetContainerInterfaceInfo returns information to configure networking for a
// container. It accepts container tags as arguments.
func (p *ProvisionerAPI) GetContainerInterfaceInfo(args params.Entities) (
	params.MachineNetworkConfigResults,
	error,
) {
	return p.prepareOrGetContainerInterfaceInfo(args, true)
}

// perContainerHandler is the interface we need to trigger processing on
// every container passed in as a list of things to process.
type perContainerHandler interface {
	// ProcessOneContainer is called once we've assured ourselves that all of
	// the access permissions are correct and the container is ready to be
	// processed.
	// netEnv is the Environment you are working, idx is the index for this
	// container (used for deciding where to store results), host is the
	// machine that is hosting the container.
	// Any errors that are returned from ProcessOneContainer will be turned
	// into ServerError and handed to SetError
	ProcessOneContainer(netEnv environs.NetworkingEnviron, idx int, host, container *state.Machine) error
	// SetError will be called whenever there is a problem with the a given
	// request. Generally this just does result.Results[i].Error = error
	// but the Result type is opaque so we can't do it ourselves.
	SetError(resultIndex int, err *params.Error)
}

func (p *ProvisionerAPI) processEachContainer(args params.Entities, handler perContainerHandler) error {
	netEnviron, hostMachine, canAccess, err := p.prepareContainerAccessEnvironment()
	if err != nil {
		// Overall error
		return errors.Trace(err)
	}
	_, err = hostMachine.InstanceId()
	if errors.IsNotProvisioned(err) {
		err = errors.NotProvisionedf("cannot prepare container network config: host machine %q", hostMachine)
		return err
	} else if err != nil {
		return errors.Trace(err)
	}

	for i, entity := range args.Entities {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			handler.SetError(i, common.ServerError(err))
			continue
		}
		// The auth function (canAccess) checks that the machine is a
		// top level machine (we filter those out next) or that the
		// machine has the host as a parent.
		container, err := p.getMachine(canAccess, machineTag)
		if err != nil {
			handler.SetError(i, common.ServerError(err))
			continue
		} else if !container.IsContainer() {
			err = errors.Errorf("cannot prepare network config for %q: not a container", machineTag)
			handler.SetError(i, common.ServerError(err))
			continue
		}

		if err := handler.ProcessOneContainer(netEnviron, i, hostMachine, container); err != nil {
			handler.SetError(i, common.ServerError(err))
			continue
		}
	}
	return nil
}

type prepareOrGetContext struct {
	result   params.MachineNetworkConfigResults
	maintain bool
}

func (ctx *prepareOrGetContext) SetError(idx int, err *params.Error) {
	ctx.result.Results[idx].Error = err
}

func (ctx *prepareOrGetContext) ProcessOneContainer(netEnv environs.NetworkingEnviron, idx int, host, container *state.Machine) error {
	containerId, err := container.InstanceId()
	if ctx.maintain {
		if err == nil {
			// Since we want to configure and create NICs on the
			// container before it starts, it must also be not
			// provisioned yet.
			return errors.Errorf("container %q already provisioned as %q", container, containerId)
		}
	}
	// The only error we allow is NotProvisioned
	if err != nil && !errors.IsNotProvisioned(err) {
		return err
	}

	if err := host.SetContainerLinkLayerDevices(container); err != nil {
		return err
	}

	containerDevices, err := container.AllLinkLayerDevices()
	if err != nil {
		return err
	}

	preparedInfo := make([]network.InterfaceInfo, len(containerDevices))
	for j, device := range containerDevices {
		parentDevice, err := device.ParentDevice()
		if err != nil || parentDevice == nil {
			return errors.Errorf(
				"cannot get parent %q of container device %q: %v",
				device.ParentName(), device.Name(), err,
			)
		}
		parentAddrs, err := parentDevice.Addresses()
		if err != nil {
			return err
		}

		info := network.InterfaceInfo{
			InterfaceName:       device.Name(),
			MACAddress:          device.MACAddress(),
			ConfigType:          network.ConfigManual,
			InterfaceType:       network.InterfaceType(device.Type()),
			NoAutoStart:         !device.IsAutoStart(),
			Disabled:            !device.IsUp(),
			MTU:                 int(device.MTU()),
			ParentInterfaceName: parentDevice.Name(),
		}

		if len(parentAddrs) > 0 {
			logger.Infof("host machine device %q has addresses %v", parentDevice.Name(), parentAddrs)

			firstAddress := parentAddrs[0]
			parentDeviceSubnet, err := firstAddress.Subnet()
			if err != nil {
				return errors.Annotatef(err,
					"cannot get subnet %q used by address %q of host machine device %q",
					firstAddress.SubnetCIDR(), firstAddress.Value(), parentDevice.Name(),
				)
			}
			info.ConfigType = network.ConfigStatic
			info.CIDR = parentDeviceSubnet.CIDR()
			info.ProviderSubnetId = parentDeviceSubnet.ProviderId()
			info.VLANTag = parentDeviceSubnet.VLANTag()
		} else {
			logger.Infof("host machine device %q has no addresses %v", parentDevice.Name(), parentAddrs)
		}

		logger.Tracef("prepared info for container interface %q: %+v", info.InterfaceName, info)
		preparedInfo[j] = info
	}

	hostInstanceId, err := host.InstanceId()
	if err != nil {
		// this should have already been checked in the processEachContainer helper
		return err
	}
	allocatedInfo, err := netEnv.AllocateContainerAddresses(hostInstanceId, container.MachineTag(), preparedInfo)
	if err != nil {
		return err
	}
	logger.Debugf("got allocated info from provider: %+v", allocatedInfo)

	allocatedConfig := networkingcommon.NetworkConfigFromInterfaceInfo(allocatedInfo)
	logger.Tracef("allocated network config: %+v", allocatedConfig)
	ctx.result.Results[idx].Config = allocatedConfig
	return nil
}

func (p *ProvisionerAPI) prepareOrGetContainerInterfaceInfo(args params.Entities, maintain bool) (params.MachineNetworkConfigResults, error) {
	ctx := &prepareOrGetContext{
		result: params.MachineNetworkConfigResults{
			Results: make([]params.MachineNetworkConfigResult, len(args.Entities)),
		},
		maintain: maintain,
	}

	if err := p.processEachContainer(args, ctx); err != nil {
		return ctx.result, errors.Trace(err)
	}
	return ctx.result, nil
}

// prepareContainerAccessEnvironment retrieves the environment, host machine, and access
// for working with containers.
func (p *ProvisionerAPI) prepareContainerAccessEnvironment() (environs.NetworkingEnviron, *state.Machine, common.AuthFunc, error) {
	netEnviron, err := networkingcommon.NetworkingEnvironFromModelConfig(p.configGetter)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
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

type hostChangesContext struct {
	result params.HostNetworkChangeResults
}

func (ctx *hostChangesContext) ProcessOneContainer(netEnv environs.NetworkingEnviron, idx int, host, container *state.Machine) error {
	bridges, err := host.FindMissingBridgesForContainer(container)
	if err != nil {
		return err
	}
	for _, bridgeInfo := range bridges {
		ctx.result.Results[idx].NewBridges = append(
			ctx.result.Results[idx].NewBridges,
			params.DeviceBridgeInfo{
				HostDeviceName: bridgeInfo.DeviceName,
				BridgeName: bridgeInfo.BridgeName,
			})
	}
	return nil
}

func (ctx *hostChangesContext) SetError(idx int, err *params.Error) {
	ctx.result.Results[idx].Error = err
}

// HostChangesForContainers returns the set of changes that need to be done
// to the host machine to prepare it for the containers to be created.
// Pass in a list of the containers that you want the changes for.
func (p *ProvisionerAPI) HostChangesForContainers(args params.Entities) (params.HostNetworkChangeResults, error) {
	ctx := &hostChangesContext{
		result: params.HostNetworkChangeResults{
			Results: make([]params.HostNetworkChange, len(args.Entities)),
		},
	}
	if err := p.processEachContainer(args, ctx); err != nil {
		return ctx.result, errors.Trace(err)
	}
	return ctx.result, nil
}

// InstanceStatus returns the instance status for each given entity.
// Only machine tags are accepted.
func (p *ProvisionerAPI) InstanceStatus(args params.Entities) (params.StatusResults, error) {
	result := params.StatusResults{
		Results: make([]params.StatusResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		logger.Errorf("failed to get an authorisation function: %v", err)
		return result, errors.Trace(err)
	}
	for i, arg := range args.Entities {
		mTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		machine, err := p.getMachine(canAccess, mTag)
		if err == nil {
			var statusInfo status.StatusInfo
			statusInfo, err = machine.InstanceStatus()
			result.Results[i].Status = statusInfo.Status.String()
			result.Results[i].Info = statusInfo.Message
			result.Results[i].Data = statusInfo.Data
			result.Results[i].Since = statusInfo.Since
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetInstanceStatus updates the instance status for each given
// entity. Only machine tags are accepted.
func (p *ProvisionerAPI) SetInstanceStatus(args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := p.getAuthFunc()
	if err != nil {
		logger.Errorf("failed to get an authorisation function: %v", err)
		return result, errors.Trace(err)
	}
	for i, arg := range args.Entities {
		mTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		machine, err := p.getMachine(canAccess, mTag)
		if err == nil {
			// TODO(perrito666) 2016-05-02 lp:1558657
			now := time.Now()
			s := status.StatusInfo{
				Status:  status.Status(arg.Status),
				Message: arg.Info,
				Data:    arg.Data,
				Since:   &now,
			}
			err = machine.SetInstanceStatus(s)
			if status.Status(arg.Status) == status.ProvisioningError {
				s.Status = status.Error
				if err == nil {
					err = machine.SetStatus(s)
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// MarkMachinesForRemoval indicates that the specified machines are
// ready to have any provider-level resources cleaned up and then be
// removed.
func (p *ProvisionerAPI) MarkMachinesForRemoval(machines params.Entities) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(machines.Entities))
	canAccess, err := p.getAuthFunc()
	if err != nil {
		logger.Errorf("failed to get an authorisation function: %v", err)
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, machine := range machines.Entities {
		results[i].Error = common.ServerError(p.markOneMachineForRemoval(machine.Tag, canAccess))
	}
	return params.ErrorResults{Results: results}, nil
}

func (p *ProvisionerAPI) markOneMachineForRemoval(machineTag string, canAccess common.AuthFunc) error {
	mTag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	machine, err := p.getMachine(canAccess, mTag)
	if err != nil {
		return errors.Trace(err)
	}
	return machine.MarkForRemoval()
}
