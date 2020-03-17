// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.instancepoller")

// InstancePollerAPI provides access to the InstancePoller API facade.
type InstancePollerAPI struct {
	*common.LifeGetter
	*common.ModelWatcher
	*common.ModelMachinesWatcher
	*common.InstanceIdGetter
	*common.StatusGetter

	st            StateInterface
	resources     facade.Resources
	authorizer    facade.Authorizer
	accessMachine common.GetAuthFunc
	clock         clock.Clock
}

// NewFacade wraps NewInstancePollerAPI for facade registration.
func NewFacade(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*InstancePollerAPI, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewInstancePollerAPI(st, m, resources, authorizer, clock.WallClock)
}

// NewInstancePollerAPI creates a new server-side InstancePoller API
// facade.
func NewInstancePollerAPI(
	st *state.State,
	m *state.Model,
	resources facade.Resources,
	authorizer facade.Authorizer,
	clock clock.Clock,
) (*InstancePollerAPI, error) {

	if !authorizer.AuthController() {
		// InstancePoller must run as a controller.
		return nil, common.ErrPerm
	}
	accessMachine := common.AuthFuncForTagKind(names.MachineTagKind)
	sti := getState(st, m)

	// Life() is supported for machines.
	lifeGetter := common.NewLifeGetter(
		sti,
		accessMachine,
	)
	// ModelConfig() and WatchForModelConfigChanges() are allowed
	// with unrestricted access.
	modelWatcher := common.NewModelWatcher(
		sti,
		resources,
		authorizer,
	)
	// WatchModelMachines() is allowed with unrestricted access.
	machinesWatcher := common.NewModelMachinesWatcher(
		sti,
		resources,
		authorizer,
	)
	// InstanceId() is supported for machines.
	instanceIdGetter := common.NewInstanceIdGetter(
		sti,
		accessMachine,
	)
	// Status() is supported for machines.
	statusGetter := common.NewStatusGetter(
		sti,
		accessMachine,
	)

	return &InstancePollerAPI{
		LifeGetter:           lifeGetter,
		ModelWatcher:         modelWatcher,
		ModelMachinesWatcher: machinesWatcher,
		InstanceIdGetter:     instanceIdGetter,
		StatusGetter:         statusGetter,
		st:                   sti,
		resources:            resources,
		authorizer:           authorizer,
		accessMachine:        accessMachine,
		clock:                clock,
	}, nil
}

func (a *InstancePollerAPI) getOneMachine(tag string, canAccess common.AuthFunc) (StateMachine, error) {
	machineTag, err := names.ParseMachineTag(tag)
	if err != nil {
		return nil, err
	}
	if !canAccess(machineTag) {
		return nil, common.ErrPerm
	}
	return a.st.Machine(machineTag.Id())
}

// SetProviderNetworkConfig updates the provider addresses for one or more
// machines.
//
// What's more, if the client request includes provider-specific IDs (e.g.
// network, subnet or address IDs), this method will also iterate any present
// link layer devices (and their addresses) and backfill any missing
// provider-specific information.
func (a *InstancePollerAPI) SetProviderNetworkConfig(req params.SetProviderNetworkConfig) (params.SetProviderNetworkConfigResults, error) {
	result := params.SetProviderNetworkConfigResults{
		Results: make([]params.SetProviderNetworkConfigResult, len(req.Args)),
	}

	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}

	spaceInfos, err := a.st.AllSpaceInfos()
	if err != nil {
		return result, err
	}

	for i, arg := range req.Args {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		newProviderAddrs, err := mapNetworkConfigsToProviderAddresses(arg.Configs, spaceInfos)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		newSpaceAddrs, err := newProviderAddrs.ToSpaceAddresses(spaceInfos)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		modified, err := maybeUpdateMachineProviderAddresses(machine, newSpaceAddrs)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		result.Results[i].Modified = modified
		result.Results[i].Addresses = params.FromProviderAddresses(newProviderAddrs...)

		// If we were able to acquire full network device information
		// from the provider which includes provider-specific IDs that
		// are not visible to the machiner worker (primary source for
		// link layer devices/addresses), we can attempt to backfill
		// the missing provider IDs.
		if containsProviderIDs(arg.Configs) {
			// Treat errors as transient; the purpose of this API
			// method is to simply update the provider addresses.
			if err := backfillProviderIDs(machine, params.InterfaceInfoFromNetworkConfig(arg.Configs)); err != nil {
				logger.Errorf("link layer device backfill attempt for machine %v failed due to error: %v; waiting until next instancepoller run to retry", machine.Id(), err)
			}
		}
	}
	return result, nil
}

func maybeUpdateMachineProviderAddresses(m StateMachine, newSpaceAddrs network.SpaceAddresses) (bool, error) {
	curSpaceAddrs := m.ProviderAddresses()
	if curSpaceAddrs.EqualTo(newSpaceAddrs) {
		return false, nil
	}

	return true, m.SetProviderAddresses(newSpaceAddrs...)
}

func backfillProviderIDs(m StateMachine, ifaces []network.InterfaceInfo) error {
	existingDevs, err := m.AllLinkLayerDevices()
	if err != nil {
		return err
	}

	// Since the device name might be different (i.e. providers
	// like AWS do not report device names) we can only reliably
	// match on the device address.
	addrToIfaceMap := make(map[string]network.InterfaceInfo)
	for _, iface := range ifaces {
		addrToIfaceMap[iface.PrimaryAddress().Value] = iface
	}

	var (
		devUpdates  []state.LinkLayerDeviceArgs
		addrUpdates []state.LinkLayerDeviceAddress
	)
	for _, existingDev := range existingDevs {
		devAddrs, err := existingDev.Addresses()
		if err != nil {
			return err
		}

		for _, addr := range devAddrs {
			iface, matched := addrToIfaceMap[addr.Value()]
			if !matched {
				continue
			}

			// Re-use the primary information persisted by the
			// machiner but populate the provider-related ID fields
			// with the data obtained by the network provider.
			devUpdates = append(devUpdates, state.LinkLayerDeviceArgs{
				Name:        existingDev.Name(),
				MTU:         existingDev.MTU(),
				ProviderID:  iface.ProviderId,
				Type:        existingDev.Type(),
				MACAddress:  existingDev.MACAddress(),
				IsAutoStart: existingDev.IsAutoStart(),
				IsUp:        existingDev.IsUp(),
				ParentName:  existingDev.ParentName(),
			})

			addrInCIDRNotation, err := network.IPToCIDRNotation(addr.Value(), addr.SubnetCIDR())
			if err != nil {
				return err
			}

			addrUpdates = append(addrUpdates, state.LinkLayerDeviceAddress{
				DeviceName:        existingDev.Name(),
				ConfigMethod:      addr.ConfigMethod(),
				ProviderID:        iface.ProviderAddressId,
				ProviderNetworkID: iface.ProviderNetworkId,
				ProviderSubnetID:  iface.ProviderSubnetId,
				CIDRAddress:       addrInCIDRNotation,
				DNSServers:        addr.DNSServers(),
				DNSSearchDomains:  addr.DNSSearchDomains(),
				GatewayAddress:    addr.GatewayAddress(),
				IsDefaultGateway:  addr.IsDefaultGateway(),
			})

			break
		}
	}

	if len(devUpdates) != 0 {
		logger.Debugf("merging the following link layer devices into device list for machine %v: %+v", m.Id(), devUpdates)
		if err := m.SetParentLinkLayerDevicesBeforeTheirChildren(devUpdates); err != nil {
			return errors.Trace(err)
		}
	}

	if len(addrUpdates) != 0 {
		logger.Debugf("merging the following link layer device addresses into address list for machine %v: %+v", m.Id(), addrUpdates)
		if err := m.SetDevicesAddressesIdempotently(addrUpdates); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// mapNetworkConfigsToProviderAddresses iterates the list of incoming network
// configuration parameters, extracts all usable private/shadow IP addresses,
// attempts to resolve each one to a known space and returns back a list of
// scoped, space-aware ProviderAddresses.
func mapNetworkConfigsToProviderAddresses(cfgs []params.NetworkConfig, spaceInfos network.SpaceInfos) (network.ProviderAddresses, error) {
	addrs := make(network.ProviderAddresses, 0, len(cfgs))

	alphaSpaceInfo := spaceInfos.GetByID(network.AlphaSpaceId)
	if alphaSpaceInfo == nil {
		return network.ProviderAddresses{}, errors.New("BUG: space infos lack entry for alpha space")
	}

	for _, cfg := range cfgs {
		// Private addresses use the same CIDR; try to resolve them
		// to a space and create a scoped network address
		for _, addr := range params.ToProviderAddresses(cfg.Addresses...) {
			spaceInfo, err := spaceInfoForAddress(spaceInfos, cfg.CIDR, cfg.ProviderSubnetId, addr.Value)
			if err != nil {
				// If we were unable to infer the space using the
				// currently available subnet information, use
				// alpha space as a fall-back
				if !errors.IsNotFound(err) {
					return network.ProviderAddresses{}, err
				}
				spaceInfo = alphaSpaceInfo
			}

			addrs = append(
				addrs,
				network.NewScopedProviderAddressInSpace(string(spaceInfo.Name), addr.Value, addr.Scope),
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
				if !errors.IsNotFound(err) {
					return network.ProviderAddresses{}, err
				}
				spaceInfo = alphaSpaceInfo
			}
			addrs = append(
				addrs,
				network.NewScopedProviderAddressInSpace(string(spaceInfo.Name), addr.Value, addr.Scope),
			)
		}
	}

	return addrs, nil
}

func containsProviderIDs(cfgs []params.NetworkConfig) bool {
	for _, cfg := range cfgs {
		if cfg.ProviderId != "" || cfg.ProviderSubnetId != "" || cfg.ProviderAddressId != "" || cfg.ProviderNetworkId != "" {
			return true
		}
	}
	return false
}

func spaceInfoForAddress(spaceInfos network.SpaceInfos, CIDR, providerSubnetID, addr string) (*network.SpaceInfo, error) {
	if CIDR != "" {
		return spaceInfos.InferSpaceFromCIDRAndSubnetID(CIDR, providerSubnetID)
	}
	return spaceInfos.InferSpaceFromAddress(addr)
}

// ProviderAddresses returns the list of all known provider addresses
// for each given entity. Only machine tags are accepted.
func (a *InstancePollerAPI) ProviderAddresses(args params.Entities) (params.MachineAddressesResults, error) {
	result := params.MachineAddressesResults{
		Results: make([]params.MachineAddressesResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		addrs, err := machine.ProviderAddresses().ToProviderAddresses(a.st)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		result.Results[i].Addresses = params.FromProviderAddresses(addrs...)

	}
	return result, nil
}

// SetProviderAddresses updates the list of known provider addresses
// for each given entity. Only machine tags are accepted.
func (a *InstancePollerAPI) SetProviderAddresses(args params.SetMachinesAddresses) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineAddresses)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.MachineAddresses {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		addrsToSet, err := params.ToProviderAddresses(arg.Addresses...).ToSpaceAddresses(a.st)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := machine.SetProviderAddresses(addrsToSet...); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// InstanceStatus returns the instance status for each given entity.
// Only machine tags are accepted.
func (a *InstancePollerAPI) InstanceStatus(args params.Entities) (params.StatusResults, error) {
	result := params.StatusResults{
		Results: make([]params.StatusResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
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
func (a *InstancePollerAPI) SetInstanceStatus(args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err == nil {
			now := a.clock.Now()
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

// AreManuallyProvisioned returns whether each given entity is
// manually provisioned or not. Only machine tags are accepted.
func (a *InstancePollerAPI) AreManuallyProvisioned(args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := a.accessMachine()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err == nil {
			result.Results[i].Result, err = machine.IsManual()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// InstancePollerAPIV3 implements the V3 API used by the instance poller
// worker. Compared to V4, it lacks the SetProviderNetworkConfig method.
type InstancePollerAPIV3 struct {
	*InstancePollerAPI
}

// NewFacadeV3 creates a new instance of the V3 InstancePoller API.
func NewFacadeV3(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*InstancePollerAPIV3, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	api, err := NewInstancePollerAPI(st, m, resources, authorizer, clock.WallClock)
	if err != nil {
		return nil, err
	}

	return &InstancePollerAPIV3{api}, nil
}

// SetProviderNetworkConfig is not available in V3.
func (*InstancePollerAPIV3) SetProviderNetworkConfig(_, _ struct{}) {}
