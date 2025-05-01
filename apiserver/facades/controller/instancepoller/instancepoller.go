// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// InstancePollerAPI provides access to the InstancePoller API facade.
type InstancePollerAPI struct {
	*common.LifeGetter
	*commonmodel.ModelMachinesWatcher
	*common.InstanceIdGetter
	*common.StatusGetter

	st                      StateInterface
	networkService          NetworkService
	machineService          MachineService
	accessMachine           common.GetAuthFunc
	controllerConfigService ControllerConfigService
	clock                   clock.Clock
	logger                  corelogger.Logger
}

// NewInstancePollerAPI creates a new server-side InstancePoller API
// facade.
func NewInstancePollerAPI(
	st *state.State,
	networkService NetworkService,
	machineService MachineService,
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
		sti,
		accessMachine,
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
	// Status() is supported for machines.
	statusGetter := common.NewStatusGetter(
		sti,
		accessMachine,
	)

	return &InstancePollerAPI{
		LifeGetter:              lifeGetter,
		ModelMachinesWatcher:    machinesWatcher,
		InstanceIdGetter:        instanceIdGetter,
		StatusGetter:            statusGetter,
		networkService:          networkService,
		machineService:          machineService,
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
			a.logger.Debugf(context.TODO(), "machine %q is not alive; skipping provider network config update", machine.Id())
			continue
		}

		configs := arg.Configs
		a.logger.Tracef(context.TODO(), "provider network config for machine %q: %+v", machine.Id(), configs)

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
		if err := a.mergeLinkLayer(machine, params.InterfaceInfoFromNetworkConfig(configs)); err != nil {
			a.logger.Errorf(context.TODO(),
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
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err == nil {
			var statusInfo status.StatusInfo
			statusInfo, err = machine.InstanceStatus()
			if err == nil {
				result.Results[i].Status = statusInfo.Status.String()
				result.Results[i].Info = statusInfo.Message
				result.Results[i].Data = statusInfo.Data
				result.Results[i].Since = statusInfo.Since
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
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
		result.Results[i].Error = apiservererrors.ServerError(err)
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
		machine, err := a.getOneMachine(arg.Tag, canAccess)
		if err == nil {
			result.Results[i].Result, err = machine.IsManual()
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
