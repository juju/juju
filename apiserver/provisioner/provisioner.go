// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/status"
)

var logger = loggo.GetLogger("juju.apiserver.provisioner")

func init() {
	common.RegisterStandardFacade("Provisioner", 1, NewProvisionerAPI)

	// Version 1 has the same set of methods as 0, with the same
	// signatures, but its ProvisioningInfo returns additional
	// information. Clients may require version 1 so that they
	// receive this additional information; otherwise they are
	// compatible.
	common.RegisterStandardFacade("Provisioner", 2, NewProvisionerAPI)
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
	*common.ModelWatcher
	*common.ModelMachinesWatcher
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
	env, err := st.Model()
	if err != nil {
		return nil, err
	}
	urlGetter := common.NewToolsURLGetter(env.UUID(), st)
	return &ProvisionerAPI{
		Remover:              common.NewRemover(st, false, getAuthFunc),
		StatusSetter:         common.NewStatusSetter(st, getAuthFunc),
		StatusGetter:         common.NewStatusGetter(st, getAuthFunc),
		DeadEnsurer:          common.NewDeadEnsurer(st, getAuthFunc),
		PasswordChanger:      common.NewPasswordChanger(st, getAuthFunc),
		LifeGetter:           common.NewLifeGetter(st, getAuthFunc),
		StateAddresser:       common.NewStateAddresser(st),
		APIAddresser:         common.NewAPIAddresser(st, resources),
		ModelWatcher:         common.NewModelWatcher(st, resources, authorizer),
		ModelMachinesWatcher: common.NewModelMachinesWatcher(st, resources, authorizer),
		InstanceIdGetter:     common.NewInstanceIdGetter(st, getAuthFunc),
		ToolsFinder:          common.NewToolsFinder(st, st, urlGetter),
		ToolsGetter:          common.NewToolsGetter(st, st, st, urlGetter, getAuthOwner),
		st:                   st,
		resources:            resources,
		authorizer:           authorizer,
		getAuthFunc:          getAuthFunc,
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
	config, err := p.st.ModelConfig()
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
	case instance.LXD:
		// TODO(jam): DefaultMTU needs to be handled here
		// TODO(jam): Do we want to handle ImageStream here, or do we
		// hide it from them? (all cached images must come from the
		// same image stream?)
	}

	if !environs.AddressAllocationEnabled(config.Type()) {
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
		result.Status = statusInfo.Status.String()
		result.Info = statusInfo.Message
		result.Data = statusInfo.Data
		if statusInfo.Status != status.StatusError {
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

func containerHostname(containerTag names.Tag) string {
	return fmt.Sprintf("%s-%s", container.DefaultNamespace, containerTag.String())
}

// ReleaseContainerAddresses finds addresses allocated to a container
// and marks them as Dead, to be released and removed. It accepts
// container tags as arguments. If address allocation feature flag is
// not enabled, it will return a NotSupported error.
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

func (p *ProvisionerAPI) legacyAddressAllocationSupported() (bool, error) {
	config, err := p.st.ModelConfig()
	if err != nil {
		return false, errors.Trace(err)
	}
	return environs.AddressAllocationEnabled(config.Type()), nil
}

// PrepareContainerInterfaceInfo allocates an address and returns
// information to configure networking for a container. It accepts
// container tags as arguments. If the address allocation feature flag
// is not enabled, it returns a NotSupported error.
func (p *ProvisionerAPI) PrepareContainerInterfaceInfo(args params.Entities) (
	params.MachineNetworkConfigResults,
	error,
) {
	supported, err := p.legacyAddressAllocationSupported()
	if err != nil {
		return params.MachineNetworkConfigResults{}, errors.Trace(err)
	}
	if supported {
		logger.Warningf("address allocation enabled - using legacyPrepareOrGetContainerInterfaceInfo(true)")
		return p.legacyPrepareOrGetContainerInterfaceInfo(args, true)
	}
	return p.prepareOrGetContainerInterfaceInfo(args, true)
}

// GetContainerInterfaceInfo returns information to configure networking
// for a container. It accepts container tags as arguments. If the address
// allocation feature flag is not enabled, it returns a NotSupported error.
func (p *ProvisionerAPI) GetContainerInterfaceInfo(args params.Entities) (
	params.MachineNetworkConfigResults,
	error,
) {
	supported, err := p.legacyAddressAllocationSupported()
	if err != nil {
		return params.MachineNetworkConfigResults{}, errors.Trace(err)
	}
	if supported {
		logger.Warningf("address allocation enabled - using legacyPrepareOrGetContainerInterfaceInfo(false)")
		return p.legacyPrepareOrGetContainerInterfaceInfo(args, false)
	}
	return p.prepareOrGetContainerInterfaceInfo(args, false)
}

// MACAddressTemplate is used to generate a unique MAC address for a
// container. Every '%x' is replaced by a random hexadecimal digit,
// while the rest is kept as-is.
const MACAddressTemplate = "00:16:3e:%02x:%02x:%02x"

// generateMACAddress creates a random MAC address within the space defined by
// MACAddressTemplate above.
//
// TODO(dimitern): We should make a best effort to ensure the MAC address we
// generate is unique at least within the current environment.
func generateMACAddress() string {
	digits := make([]interface{}, 3)
	for i := range digits {
		digits[i] = rand.Intn(256)
	}
	return fmt.Sprintf(MACAddressTemplate, digits...)
}

func (p *ProvisionerAPI) prepareOrGetContainerInterfaceInfo(args params.Entities, maintain bool) (params.MachineNetworkConfigResults, error) {
	result := params.MachineNetworkConfigResults{
		Results: make([]params.MachineNetworkConfigResult, len(args.Entities)),
	}

	netEnviron, hostMachine, canAccess, err := p.prepareContainerAccessEnvironment()
	if err != nil {
		return result, errors.Trace(err)
	}
	instId, err := hostMachine.InstanceId()
	if errors.IsNotProvisioned(err) {
		err = errors.NotProvisionedf("cannot prepare container network config: host machine %q", hostMachine)
		return result, err
	} else if err != nil {
		return result, errors.Trace(err)
	}

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
			err = errors.Errorf("cannot prepare network config for %q: not a container", tag)
			result.Results[i].Error = common.ServerError(err)
			continue
		} else if ciid, cerr := container.InstanceId(); maintain == true && cerr == nil {
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

		if err := hostMachine.SetContainerLinkLayerDevices(container); err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		containerDevices, err := container.AllLinkLayerDevices()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		preparedInfo := make([]network.InterfaceInfo, len(containerDevices))
		preparedOK := true
		for j, device := range containerDevices {
			parentDevice, err := device.ParentDevice()
			if err != nil || parentDevice == nil {
				err = errors.Errorf(
					"cannot get parent %q of container device %q: %v",
					device.ParentName(), device.Name(), err,
				)
				result.Results[i].Error = common.ServerError(err)
				preparedOK = false
				break
			}
			parentAddrs, err := parentDevice.Addresses()
			if err != nil {
				result.Results[i].Error = common.ServerError(err)
				preparedOK = false
				break
			}
			if len(parentAddrs) == 0 {
				err = errors.Errorf("host machine device %q has no addresses", parentDevice.Name())
				result.Results[i].Error = common.ServerError(err)
				preparedOK = false
				break
			}
			firstAddress := parentAddrs[0]
			parentDeviceSubnet, err := firstAddress.Subnet()
			if err != nil {
				err = errors.Annotatef(err,
					"cannot get subnet %q used by address %q of host machine device %q",
					firstAddress.SubnetCIDR(), firstAddress.Value(), parentDevice.Name(),
				)
				result.Results[i].Error = common.ServerError(err)
				preparedOK = false
				break
			}

			info := network.InterfaceInfo{
				InterfaceName:       device.Name(),
				MACAddress:          device.MACAddress(),
				ConfigType:          network.ConfigStatic,
				InterfaceType:       network.InterfaceType(device.Type()),
				NoAutoStart:         !device.IsAutoStart(),
				Disabled:            !device.IsUp(),
				MTU:                 int(device.MTU()),
				CIDR:                parentDeviceSubnet.CIDR(),
				ProviderSubnetId:    parentDeviceSubnet.ProviderId(),
				VLANTag:             parentDeviceSubnet.VLANTag(),
				ParentInterfaceName: parentDevice.Name(),
			}
			logger.Tracef("prepared info for container interface %q: %+v", info.InterfaceName, info)
			preparedOK = true
			preparedInfo[j] = info
		}

		if !preparedOK {
			// Error result is already set.
			continue
		}

		allocatedInfo, err := netEnviron.AllocateContainerAddresses(instId, preparedInfo)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		logger.Debugf("got allocated info from provider: %+v", allocatedInfo)

		allocatedConfig := networkingcommon.NetworkConfigFromInterfaceInfo(allocatedInfo)
		sortedAllocatedConfig := networkingcommon.SortNetworkConfigsByInterfaceName(allocatedConfig)
		logger.Tracef("allocated sorted network config: %+v", sortedAllocatedConfig)
		result.Results[i].Config = sortedAllocatedConfig
	}
	return result, nil
}

// legacyPrepareOrGetContainerInterfaceInfo optionally allocates an address and
// returns information for configuring networking on a container. It accepts
// container tags as arguments.
func (p *ProvisionerAPI) legacyPrepareOrGetContainerInterfaceInfo(
	args params.Entities,
	provisionContainer bool,
) (
	params.MachineNetworkConfigResults,
	error,
) {
	result := params.MachineNetworkConfigResults{
		Results: make([]params.MachineNetworkConfigResult, len(args.Entities)),
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
	var subnet *state.Subnet
	var subnetInfo network.SubnetInfo
	var interfaceInfo network.InterfaceInfo
	if environs.AddressAllocationEnabled(environ.Config().Type()) {
		// We don't need a subnet unless we need to allocate a static IP.
		subnet, subnetInfo, interfaceInfo, err = p.prepareAllocationNetwork(environ, instId)
		if err != nil {
			return result, errors.Annotate(err, "cannot allocate addresses")
		}
	} else {
		var allInterfaceInfos []network.InterfaceInfo
		allInterfaceInfos, err = environ.NetworkInterfaces(instId)
		if err != nil {
			return result, errors.Annotatef(err, "cannot instance %q interfaces", instId)
		} else if len(allInterfaceInfos) == 0 {
			return result, errors.New("no interfaces available")
		}
		// Currently we only support a single NIC per container, so we only need
		// the information from the host instance's first NIC.
		logger.Tracef("interfaces for instance %q: %v", instId, allInterfaceInfos)
		interfaceInfo = allInterfaceInfos[0]
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
			// Allocate and set an address.
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
		for l, dns := range interfaceInfo.DNSServers {
			dnsServers[l] = dns.Value
		}

		if macAddress == "" {
			macAddress = interfaceInfo.MACAddress
		}

		interfaceType := string(interfaceInfo.InterfaceType)
		if interfaceType == "" {
			interfaceType = string(network.EthernetInterface)
		}

		result.Results[i] = params.MachineNetworkConfigResult{
			Config: []params.NetworkConfig{{
				DeviceIndex:      interfaceInfo.DeviceIndex,
				MACAddress:       macAddress,
				CIDR:             subnetInfo.CIDR,
				ProviderId:       string(interfaceInfo.ProviderId),
				ProviderSubnetId: string(subnetInfo.ProviderId),
				VLANTag:          interfaceInfo.VLANTag,
				InterfaceType:    interfaceType,
				InterfaceName:    interfaceInfo.InterfaceName,
				Disabled:         interfaceInfo.Disabled,
				NoAutoStart:      interfaceInfo.NoAutoStart,
				DNSServers:       dnsServers,
				ConfigType:       string(network.ConfigStatic),
				Address:          address.Value(),
				GatewayAddress:   interfaceInfo.GatewayAddress.Value,
			}},
		}
	}
	return result, nil
}

// prepareContainerAccessEnvironment retrieves the environment, host machine, and access
// for working with containers.
func (p *ProvisionerAPI) prepareContainerAccessEnvironment() (environs.NetworkingEnviron, *state.Machine, common.AuthFunc, error) {
	netEnviron, err := networkingcommon.NetworkingEnvironFromModelConfig(p.st)
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

// prepareAllocationNetwork retrieves the subnet, its info, and the interface info
// for the allocations.
func (p *ProvisionerAPI) prepareAllocationNetwork(
	environ environs.NetworkingEnviron,
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
		return nil, subnetInfo, interfaceInfo, errors.New("no interfaces available")
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
		if sub.AllocatableIPLow != nil && sub.AllocatableIPLow.To4() == nil {
			logger.Tracef("ignoring IPv6 subnet %q - allocating IPv6 addresses not yet supported", sub.ProviderId)
			// Until we change the way we pick addresses, IPv6 subnets with
			// their *huge* ranges (/64 being the default), there is no point in
			// allowing such subnets (it won't even work as PickNewAddress()
			// assumes IPv4 allocatable range anyway).
			continue
		}
		ok, err := environ.SupportsAddressAllocation(sub.ProviderId)
		if err == nil && ok {
			subnetInfo = sub
			interfaceInfo = subnetIdToInterface[sub.ProviderId]

			// Since with addressable containers the host acts like a gateway
			// for the containers, instead of using the same gateway for the
			// containers as their host's
			interfaceInfo.GatewayAddress.Value = interfaceInfo.Address.Value

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
	hostname := containerHostname(container.Tag())

	if !environs.AddressAllocationEnabled(environ.Config().Type()) {
		// Even if the address allocation feature flag is not enabled, we might
		// be running on MAAS 1.8+ with devices support, which we can use to
		// register containers getting IPs via DHCP. However, most of the usual
		// allocation code can be bypassed, we just need the parent instance ID
		// and a MAC address (no subnet or IP address).
		allocatedAddress := network.Address{}
		err := environ.AllocateAddress(instId, network.AnySubnet, &allocatedAddress, macAddress, hostname)
		if err != nil {
			// Not using MAAS 1.8+ or some other error.
			return nil, errors.Trace(err)
		}

		logger.Infof(
			"allocated address %q on instance %q for container %q",
			allocatedAddress.String(), instId, hostname,
		)

		// Add the address to state, so we can look it up later by MAC address.
		stateAddr, err := p.st.AddIPAddress(allocatedAddress, string(network.AnySubnet))
		if err != nil {
			return nil, errors.Annotatef(err, "failed to save address %q", allocatedAddress)
		}

		err = p.setAllocatedOrRelease(stateAddr, environ, instId, container, network.AnySubnet, macAddress)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return stateAddr, nil
	}

	subnetId := network.Id(subnet.ProviderId())
	for {
		addr, err := subnet.PickNewAddress()
		if err != nil {
			return nil, err
		}
		netAddr := addr.Address()
		logger.Tracef("picked new address %q on subnet %q", addr.String(), subnetId)
		// Attempt to allocate with environ.
		err = environ.AllocateAddress(instId, subnetId, &netAddr, macAddress, hostname)
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
		err = environ.ReleaseAddress(instId, subnetId, addr.Address(), addr.MACAddress(), "")
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
		ProviderId:        subnetInfo.ProviderId,
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
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
