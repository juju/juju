// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ControllerConfigService defines the methods on the controller config service
// that are needed by the machiner API.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
	// AddSubnet creates and returns a new subnet.
	AddSubnet(ctx context.Context, args network.SubnetInfo) (network.Id, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// EnsureDeadMachine sets the provided machine's life status to Dead.
	// No error is returned if the provided machine doesn't exist, just nothing
	// gets updated.
	EnsureDeadMachine(ctx context.Context, machineName machine.Name) error
	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(context.Context, machine.Name) (bool, error)
}

// MachinerAPI implements the API used by the machiner worker.
type MachinerAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser
	*networkingcommon.NetworkConfigAPI

	networkService          NetworkService
	machineService          MachineService
	st                      *state.State
	controllerConfigService ControllerConfigService
	auth                    facade.Authorizer
	getCanModify            common.GetAuthFunc
	getCanRead              common.GetAuthFunc
}

// MachinerAPI5 stubs out the Jobs() and SetMachineAddresses() methods.
type MachinerAPIv5 struct {
	*MachinerAPI
}

// NewMachinerAPIForState creates a new instance of the Machiner API.
func NewMachinerAPIForState(
	ctx context.Context,
	ctrlSt, st *state.State,
	controllerConfigService ControllerConfigService,
	cloudService common.CloudService,
	networkService NetworkService,
	machineService MachineService,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*MachinerAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	getCanAccess := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}

	netConfigAPI, err := networkingcommon.NewNetworkConfigAPI(ctx, st, cloudService, networkService, getCanAccess)
	if err != nil {
		return nil, errors.Annotate(err, "instantiating network config API")
	}

	return &MachinerAPI{
		LifeGetter:              common.NewLifeGetter(st, getCanAccess),
		StatusSetter:            common.NewStatusSetter(st, getCanAccess),
		DeadEnsurer:             common.NewDeadEnsurer(st, getCanAccess, machineService),
		AgentEntityWatcher:      common.NewAgentEntityWatcher(st, resources, getCanAccess),
		APIAddresser:            common.NewAPIAddresser(ctrlSt, resources),
		NetworkConfigAPI:        netConfigAPI,
		networkService:          networkService,
		machineService:          machineService,
		st:                      st,
		controllerConfigService: controllerConfigService,
		auth:                    authorizer,
		getCanModify:            getCanAccess,
		getCanRead:              getCanAccess,
	}, nil
}

func (api *MachinerAPI) getMachine(tag string, authChecker common.AuthFunc) (*state.Machine, error) {
	mtag, err := names.ParseMachineTag(tag)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	} else if !authChecker(mtag) {
		return nil, apiservererrors.ErrPerm
	}

	entity, err := api.st.FindEntity(mtag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Machine), nil
}

func (api *MachinerAPI) SetMachineAddresses(ctx context.Context, args params.SetMachinesAddresses) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineAddresses)),
	}
	canModify, err := api.getCanModify()
	if err != nil {
		return results, err
	}
	controllerConfig, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return results, err
	}
	allSpaces, err := api.networkService.GetAllSpaces(ctx)
	if err != nil {
		return results, apiservererrors.ServerError(err)
	}
	for i, arg := range args.MachineAddresses {
		m, err := api.getMachine(arg.Tag, canModify)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		pas := params.ToProviderAddresses(arg.Addresses...)
		addresses, err := pas.ToSpaceAddresses(allSpaces)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := m.SetMachineAddresses(controllerConfig, addresses...); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// SetMachineAddresses is not supported in MachinerAPI at version 5.
func (api *MachinerAPIv5) SetMachineAddresses(ctx context.Context, args params.SetMachinesAddresses) (params.ErrorResults, error) {
	return params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineAddresses)),
	}, nil
}

// Jobs is not supported in MachinerAPI at version 5.
// Deprecated: Jobs is being deprecated. Use IsController instead.
func (api *MachinerAPIv5) Jobs(ctx context.Context, args params.Entities) (params.JobsResults, error) {
	results := params.JobsResults{
		Results: make([]params.JobsResult, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}

		isController, err := api.machineService.IsMachineController(ctx, machine.Name(machineTag.Id()))
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		jobs := []string{"host-units"}
		if isController {
			jobs = append(jobs, "api-server")
		}

		results.Results[i].Jobs = jobs
	}
	return results, nil
}

// IsController returns if the given machine is a controller machine.
func (api *MachinerAPI) IsController(ctx context.Context, args params.Entities) (params.IsControllerResults, error) {
	results := params.IsControllerResults{
		Results: make([]params.IsControllerResult, len(args.Entities)),
	}

	for i, entity := range args.Entities {
		result := params.IsControllerResult{}

		// Assert that the entity is a machine.
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			// ParseMachineTag will return an InvalidTagError if the given
			// entity is not a machine.
			result.Error = apiservererrors.ServerError(err)
			results.Results[i] = result
			continue
		}
		machineName := machine.Name(machineTag.Id())

		// Check if the machine is a controller by using the machine service.
		isController, err := api.machineService.IsMachineController(ctx, machineName)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
			results.Results[i] = result
			continue
		}

		result.IsController = isController
		results.Results[i] = result
	}
	return results, nil
}

// RecordAgentStartTime updates the agent start time field in the machine doc.
func (api *MachinerAPI) RecordAgentStartTime(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canModify, err := api.getCanModify()
	if err != nil {
		return results, err
	}

	for i, entity := range args.Entities {
		m, err := api.getMachine(entity.Tag, canModify)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := m.RecordAgentStartInformation(""); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// RecordAgentStartInformation syncs the machine model with information
// reported by a machine agent when it starts.
func (api *MachinerAPI) RecordAgentStartInformation(ctx context.Context, args params.RecordAgentStartInformationArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	canModify, err := api.getCanModify()
	if err != nil {
		return results, err
	}

	for i, arg := range args.Args {
		m, err := api.getMachine(arg.Tag, canModify)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := m.RecordAgentStartInformation(arg.Hostname); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// APIHostPorts returns the API server addresses.
func (api *MachinerAPI) APIHostPorts(ctx context.Context) (result params.APIHostPortsResult, err error) {
	controllerConfig, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	return api.APIAddresser.APIHostPorts(ctx, controllerConfig)
}

// APIAddresses returns the list of addresses used to connect to the API.
func (api *MachinerAPI) APIAddresses(ctx context.Context) (result params.StringsResult, err error) {
	controllerConfig, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	return api.APIAddresser.APIAddresses(ctx, controllerConfig)
}
