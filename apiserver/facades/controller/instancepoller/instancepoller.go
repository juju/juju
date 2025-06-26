// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// InstancePollerAPI provides access to the InstancePoller API facade.
type InstancePollerAPI struct {
	*common.LifeGetter
	*commonmodel.ModelMachinesWatcher
	*common.InstanceIdGetter

	st                      StateInterface
	networkService          NetworkService
	machineService          MachineService
	statusService           StatusService
	accessMachine           common.GetAuthFunc
	controllerConfigService ControllerConfigService
	clock                   clock.Clock
	logger                  corelogger.Logger
}

// NewInstancePollerAPI creates a new server-side InstancePoller API
// facade.
func NewInstancePollerAPI(
	st *state.State,
	applicationService ApplicationService,
	networkService NetworkService,
	machineService MachineService,
	statusService StatusService,
	m *state.Model,
	resources facade.Resources,
	authorizer facade.Authorizer,
	controllerConfigService ControllerConfigService,
	clock clock.Clock,
	logger corelogger.Logger,
) (*InstancePollerAPI, error) {

	if !authorizer.AuthController() {
		// InstancePoller must run as a controller.
		return nil, apiservererrors.ErrPerm
	}
	accessMachine := common.AuthFuncForTagKind(names.MachineTagKind)
	sti := getState(st, m)

	// Life() is supported for machines.
	lifeGetter := common.NewLifeGetter(
		applicationService,
		machineService,
		sti,
		accessMachine,
		logger,
	)
	// WatchModelMachines() is allowed with unrestricted access.
	machinesWatcher := commonmodel.NewModelMachinesWatcher(
		sti,
		resources,
		authorizer,
	)
	// InstanceId() is supported for machines.
	instanceIdGetter := common.NewInstanceIdGetter(
		machineService,
		accessMachine,
	)

	return &InstancePollerAPI{
		LifeGetter:              lifeGetter,
		ModelMachinesWatcher:    machinesWatcher,
		InstanceIdGetter:        instanceIdGetter,
		networkService:          networkService,
		machineService:          machineService,
		statusService:           statusService,
		st:                      sti,
		accessMachine:           accessMachine,
		controllerConfigService: controllerConfigService,
		clock:                   clock,
		logger:                  logger,
	}, nil
}

func (a *InstancePollerAPI) getOneMachine(tag string, canAccess common.AuthFunc) (StateMachine, error) {
	machineTag, err := names.ParseMachineTag(tag)
	if err != nil {
		return nil, err
	}
	if !canAccess(machineTag) {
		return nil, apiservererrors.ErrPerm
	}
	return a.st.Machine(machineTag.Id())
}

// SetProviderNetworkConfig updates the provider addresses for one or more
// machines.
//
// What's more, if the client request includes provider-specific IDs (e.g.
// network, subnet or address IDs), this method will also iterate any present
// link layer devices (and their addresses) and merge in any missing
// provider-specific information.
func (a *InstancePollerAPI) SetProviderNetworkConfig(
	ctx context.Context,
	req params.SetProviderNetworkConfig,
) (params.SetProviderNetworkConfigResults, error) {
	result := params.SetProviderNetworkConfigResults{
		Results: make([]params.SetProviderNetworkConfigResult, len(req.Args)),
	}

	canAccess, err := a.accessMachine(ctx)
	if err != nil {
		return result, err
	}

	spaceInfos, err := a.networkService.GetAllSpaces(ctx)
	if err != nil {
		return result, err
	}

	for i, arg := range req.Args {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// We assert in transactions that the machine is alive.
		// If it is not, we assume that it will be removed from the
		// instance-poller worker subsequently.
		if machine.Life() != state.Alive {
			a.logger.Debugf(ctx, "machine %q is not alive; skipping provider network config update", machine.Id())
			continue
		}

		configs := arg.Configs
		a.logger.Tracef(ctx, "provider network config for machine %q: %+v", machine.Id(), configs)

		newProviderAddrs, err := mapNetworkConfigsToProviderAddresses(configs, spaceInfos)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		newSpaceAddrs, err := newProviderAddrs.ToSpaceAddresses(spaceInfos)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		modified, err := maybeUpdateMachineProviderAddresses(ctx, a.controllerConfigService, machine, newSpaceAddrs)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].Modified = modified
		result.Results[i].Addresses = params.FromProviderAddresses(newProviderAddrs...)

		// Treat errors as transient; the purpose of this API
		// method is to simply update the provider addresses.
		interfaceInfos := params.InterfaceInfoFromNetworkConfig(configs)
		if err := a.mergeLinkLayer(machine, interfaceInfos); err != nil {
			a.logger.Errorf(ctx,
				"link layer device merge attempt for machine %v failed due to error: %v; "+
					"waiting until next instance-poller run to retry", machine.Id(), err)
		}

		// Write in dqlite
		if err := a.setProviderConfigOneMachine(ctx, arg.Tag, interfaceInfos); err != nil {
			a.logger.Errorf(ctx,
				"link layer device merge attempt for machine %v failed due to error: %v; "+
					"waiting until next instance-poller run to retry", machine.Id(), err)
		}
	}

	return result, nil
}

func maybeUpdateMachineProviderAddresses(
	ctx context.Context,
	controllerConfigService ControllerConfigService,
	m StateMachine,
	newSpaceAddrs network.SpaceAddresses) (bool, error) {
	curSpaceAddrs := m.ProviderAddresses()
	if curSpaceAddrs.EqualTo(newSpaceAddrs) {
		return false, nil
	}

	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return false, errors.Trace(err)
	}

	return true, m.SetProviderAddresses(controllerConfig, newSpaceAddrs...)
}

func (a *InstancePollerAPI) mergeLinkLayer(m StateMachine, devs network.InterfaceInfos) error {
	return errors.Trace(a.st.ApplyOperation(newMergeMachineLinkLayerOp(m, devs, a.logger)))
}

// mapNetworkConfigsToProviderAddresses iterates the list of incoming network
// configuration parameters, extracts all usable private/shadow IP addresses,
// attempts to resolve each one to a known space and returns a list of scoped,
// space-aware ProviderAddresses.
func mapNetworkConfigsToProviderAddresses(
	cfgs []params.NetworkConfig, spaceInfos network.SpaceInfos,
) (network.ProviderAddresses, error) {
	addrs := make(network.ProviderAddresses, 0, len(cfgs))

	alphaSpaceInfo := spaceInfos.GetByID(network.AlphaSpaceId)
	if alphaSpaceInfo == nil {
		return network.ProviderAddresses{}, errors.New("BUG: space infos lack entry for alpha space")
	}

	for _, cfg := range cfgs {
		// Private addresses use the same CIDR; try to resolve them
		// to a space and create a scoped network address
		for _, addr := range params.ToProviderAddresses(cfg.Addresses...) {
			spaceInfo, err := spaceInfoForAddress(spaceInfos, addr.CIDR, cfg.ProviderSubnetId, addr.Value)
			if err != nil {
				// If we were unable to infer the space using the
				// currently available subnet information, use
				// alpha space as a fall-back
				if !errors.Is(err, errors.NotFound) {
					return network.ProviderAddresses{}, err
				}
				spaceInfo = alphaSpaceInfo
			}

			addrs = append(
				addrs,
				network.NewMachineAddress(
					addr.Value,
					network.WithScope(addr.Scope),
				).AsProviderAddress(network.WithSpaceName(string(spaceInfo.Name))),
			)
		}

		for _, addr := range params.ToProviderAddresses(cfg.ShadowAddresses...) {
			// Try to infer space from the address value only; The CIDR
			// information from cfg does not apply to these addresses.
			spaceInfo, err := spaceInfoForAddress(spaceInfos, "", "", addr.Value)
			if err != nil {
				// Space inference will always fail for public shadow addresses.
				// For those cases we auto-assign the alpha space. In the
				// future we might want to consider defining a public-alpha
				// space.
				if !errors.Is(err, errors.NotFound) {
					return network.ProviderAddresses{}, err
				}
				spaceInfo = alphaSpaceInfo
			}
			addrs = append(
				addrs,
				network.NewMachineAddress(
					addr.Value,
					network.WithScope(addr.Scope),
				).AsProviderAddress(network.WithSpaceName(string(spaceInfo.Name))),
			)
		}
	}

	return addrs, nil
}

func spaceInfoForAddress(
	spaceInfos network.SpaceInfos, cidr, providerSubnetID, addr string,
) (*network.SpaceInfo, error) {
	if cidr != "" {
		return spaceInfos.InferSpaceFromCIDRAndSubnetID(cidr, providerSubnetID)
	}
	return spaceInfos.InferSpaceFromAddress(addr)
}

// Status returns the status of the specified machine.
func (s *InstancePollerAPI) Status(ctx context.Context, args params.Entities) (params.StatusResults, error) {
	results := params.StatusResults{
		Results: make([]params.StatusResult, len(args.Entities)),
	}
	canAccess, err := s.accessMachine(ctx)
	if err != nil {
		return results, err
	}
	for i, arg := range args.Entities {
		mTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(mTag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineName := machine.Name(mTag.Id())

		statusInfo, err := s.statusService.GetMachineStatus(ctx, machineName)
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i] = params.StatusResult{
			Status: statusInfo.Status.String(),
			Info:   statusInfo.Message,
			Data:   statusInfo.Data,
			Since:  statusInfo.Since,
		}
	}
	return results, nil
}

// InstanceStatus returns the instance status for each given entity.
// Only machine tags are accepted.
func (a *InstancePollerAPI) InstanceStatus(ctx context.Context, args params.Entities) (params.StatusResults, error) {
	result := params.StatusResults{
		Results: make([]params.StatusResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine(ctx)
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(machineTag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineName := machine.Name(machineTag.Id())
		statusInfo, err := a.statusService.GetInstanceStatus(ctx, machineName)
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Status = statusInfo.Status.String()
		result.Results[i].Info = statusInfo.Message
		result.Results[i].Data = statusInfo.Data
		result.Results[i].Since = statusInfo.Since
	}
	return result, nil
}

// SetInstanceStatus updates the instance status for each given entity.
// Only machine tags are accepted.
func (a *InstancePollerAPI) SetInstanceStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine(ctx)
	if err != nil {
		return result, err
	}
	now := a.clock.Now()
	for i, arg := range args.Entities {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(machineTag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineName := machine.Name(machineTag.Id())

		s := status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
			Since:   &now,
		}
		err = a.statusService.SetInstanceStatus(ctx, machineName, s)
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if arg.Status == status.ProvisioningError.String() || arg.Status == status.Error.String() {
			s.Status = status.Error
			err := a.statusService.SetMachineStatus(ctx, machineName, s)
			if errors.Is(err, machineerrors.MachineNotFound) {
				result.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
				continue
			} else if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
		}
	}
	return result, nil
}

// AreManuallyProvisioned returns whether each given entity is
// manually provisioned or not. Only machine tags are accepted.
func (a *InstancePollerAPI) AreManuallyProvisioned(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine(ctx)
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canAccess(machineTag) {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		manual, err := a.machineService.IsMachineManuallyProvisioned(ctx, machine.Name(machineTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("machine %q", machineTag.Id()))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = manual
	}
	return result, nil
}

// setProviderConfigOneMachine sets the provider network configuration for a
// single machine given its tag and network info.
func (a *InstancePollerAPI) setProviderConfigOneMachine(
	ctx context.Context,
	machineTag string,
	devs network.InterfaceInfos,
) error {
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return internalerrors.Errorf("failed to parse machine tag %q: %w", machineTag, err)
	}
	uuid, err := a.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
	if err != nil {
		return internalerrors.Errorf("failed to get machine uuid for %q: %w", tag.Id(), err)
	}

	devices := transform.Slice(devs, newNetInterface)
	return a.networkService.SetProviderNetConfig(ctx, uuid, devices)
}

func newNetInterface(device network.InterfaceInfo) domainnetwork.NetInterface {
	return domainnetwork.NetInterface{
		Name:             device.InterfaceName,
		MTU:              ptr(int64(device.MTU)),
		MACAddress:       ptr(device.MACAddress),
		ProviderID:       ptr(device.ProviderId),
		Type:             network.LinkLayerDeviceType(device.ConfigType),
		VirtualPortType:  device.VirtualPortType,
		IsAutoStart:      !device.NoAutoStart,
		IsEnabled:        !device.Disabled,
		ParentDeviceName: device.ParentInterfaceName,
		GatewayAddress:   ptr(device.GatewayAddress.Value),
		IsDefaultGateway: device.IsDefaultGateway,
		VLANTag:          uint64(device.VLANTag),
		DNSSearchDomains: device.DNSSearchDomains,
		DNSAddresses:     device.DNSServers,
		Addrs: append(
			transform.Slice(device.Addresses, newNetAddress(device, false)),
			transform.Slice(device.ShadowAddresses, newNetAddress(device, true))...),
	}
}

func newNetAddress(device network.InterfaceInfo, isShadow bool) func(network.ProviderAddress) domainnetwork.NetAddr {
	return func(providerAddr network.ProviderAddress) domainnetwork.NetAddr {
		return domainnetwork.NetAddr{
			InterfaceName:    device.InterfaceName,
			AddressValue:     providerAddr.Value,
			AddressType:      providerAddr.Type,
			ConfigType:       providerAddr.ConfigType,
			Origin:           network.OriginProvider,
			Scope:            providerAddr.Scope,
			IsSecondary:      providerAddr.IsSecondary,
			IsShadow:         isShadow,
			ProviderID:       ptr(device.ProviderAddressId),
			ProviderSubnetID: ptr(device.ProviderSubnetId),
		}
	}
}

func ptr[T comparable](f T) *T {
	var zero T
	if f == zero {
		return nil
	}
	return &f
}
