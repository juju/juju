// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonnetwork "github.com/juju/juju/apiserver/common/network"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/rpc/params"
)

// ControllerConfigService defines the methods on the controller config service
// that are needed by the machiner API.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ControllerNodeService defines the methods on the controller node service
// that are needed by APIAddresser used by the machiner API.
type ControllerNodeService interface {
	// GetAPIAddressesByControllerIDForAgents returns a map of controller IDs to
	// their API addresses that are available for agents. The map is keyed by
	// controller ID, and the values are slices of strings representing the API
	// addresses for each controller node.
	GetAPIAddressesByControllerIDForAgents(ctx context.Context) (map[string][]string, error)
	// GetAllAPIAddressesForAgents returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
	// GetAPIHostPortsForAgents returns API HostPorts that are available for
	// agents. HostPorts are grouped by controller node, though each specific
	// controller is not identified.
	GetAPIHostPortsForAgents(ctx context.Context) ([]network.HostPorts, error)
	// WatchControllerAPIAddresses returns a watcher that observes changes to the
	// controller ip addresses.
	WatchControllerAPIAddresses(context.Context) (watcher.NotifyWatcher, error)
}

// NetworkService describes the service for working with networking concerns.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
	// AddSubnet creates and returns a new subnet.
	AddSubnet(ctx context.Context, args network.SubnetInfo) (network.Id, error)
	// SetMachineNetConfig updates the detected network configuration for
	// the machine with the input UUID.
	SetMachineNetConfig(ctx context.Context, mUUID machine.UUID, nics []domainnetwork.NetInterface) error
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(context.Context, machine.Name) (bool, error)
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)
	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(ctx context.Context, mUUID machine.UUID) (instance.Id, error)
	// GetMachineLife returns the lifecycle state of the machine with the
	// specified name.
	GetMachineLife(ctx context.Context, name machine.Name) (life.Value, error)
	// SetMachineHostname sets the hostname for the given machine.
	SetMachineHostname(ctx context.Context, mUUID machine.UUID, hostname string) error
	// WatchMachineLife returns a watcher that observes the changes to life of
	// one machine.
	WatchMachineLife(ctx context.Context, name machine.Name) (watcher.NotifyWatcher, error)
}

// ApplicationService defines the methods that the facade assumes from the
// Application service.
type ApplicationService interface {
	// GetUnitLife returns the lifecycle state of the unit with the
	// specified name.
	GetUnitLife(ctx context.Context, name unit.Name) (life.Value, error)
	// GetApplicationLifeByName looks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)
}

// StatusService defines the methods that the facade assumes from the Status
// service.
type StatusService interface {
	// SetMachineStatus sets the status of the specified machine.
	SetMachineStatus(context.Context, machine.Name, status.StatusInfo) error
}

// RemovalService defines the methods that the facade assumes from the Removal
// service.
type RemovalService interface {
	// MarkMachineAsDead marks the machine as dead. It will not remove the machine as
	// that is a separate operation. This will advance the machines's life to dead
	// and will not allow it to be transitioned back to alive.
	// Returns an error if the machine does not exist.
	MarkMachineAsDead(context.Context, machine.UUID) error
}

// ModelInfoService is the interface that is used to ask questions about the
// current model.
type ModelInfoService interface {
	// GetModelCloudType returns the type of the cloud that is in use by this model.
	GetModelCloudType(context.Context) (string, error)
}

// MachinerAPI implements the API used by the machiner worker.
type MachinerAPI struct {
	*common.LifeGetter
	*common.APIAddresser

	networkService          NetworkService
	machineService          MachineService
	statusService           StatusService
	removalService          RemovalService
	controllerConfigService ControllerConfigService
	auth                    facade.Authorizer
	getCanModify            common.GetAuthFunc
	getCanRead              common.GetAuthFunc
	watcherRegistry         facade.WatcherRegistry
	clock                   clock.Clock
}

// MachinerAPIv5 stubs out the Jobs() and SetMachineAddresses() methods.
type MachinerAPIv5 struct {
	*MachinerAPI
}

// NewMachinerAPIForState creates a new instance of the Machiner API.
func NewMachinerAPIForState(
	clock clock.Clock,
	controllerConfigService ControllerConfigService,
	controllerNodeService ControllerNodeService,
	networkService NetworkService,
	applicationService ApplicationService,
	machineService MachineService,
	statusService StatusService,
	removalService RemovalService,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	logger logger.Logger,
) (*MachinerAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	getCanAccess := func(context.Context) (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}

	return &MachinerAPI{
		LifeGetter:              common.NewLifeGetter(applicationService, machineService, getCanAccess, logger),
		APIAddresser:            common.NewAPIAddresser(controllerNodeService, watcherRegistry),
		networkService:          networkService,
		machineService:          machineService,
		statusService:           statusService,
		removalService:          removalService,
		controllerConfigService: controllerConfigService,
		auth:                    authorizer,
		getCanModify:            getCanAccess,
		getCanRead:              getCanAccess,
		watcherRegistry:         watcherRegistry,
		clock:                   clock,
	}, nil
}

// EnsureDead marks the specified machines as dead.
func (api *MachinerAPI) EnsureDead(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return results, nil
	}
	canModify, err := api.getCanModify(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canModify(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		machineUUID, err := api.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", tag.Id())
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		err = api.removalService.MarkMachineAsDead(ctx, machineUUID)
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", tag.Id())
			continue
		} else if errors.Is(err, removalerrors.MachineHasContainers) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeMachineHasContainers, "machine %q hosts containers", tag.Id())
			continue
		} else if errors.Is(err, removalerrors.MachineHasUnits) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeHasAssignedUnits, "machine %q hosts units", tag.Id())
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}

// SetObservedNetworkConfig updates network interfaces
// and IP addresses for a single machine.
func (api *MachinerAPI) SetObservedNetworkConfig(ctx context.Context, args params.SetMachineNetworkConfig) error {
	canModify, err := api.getCanModify(ctx)
	if err != nil {
		return err
	}

	mTag, err := names.ParseMachineTag(args.Tag)
	if err != nil {
		return apiservererrors.ErrPerm
	}

	if !canModify(mTag) {
		return apiservererrors.ErrPerm
	}

	mUUID, err := api.machineService.GetMachineUUID(ctx, machine.Name(mTag.Id()))
	if errors.Is(err, machineerrors.MachineNotFound) {
		// For some reason we return ErrPerm here rather than NotFound.
		// This is likely a historical artifact, but we should keep it for
		// compatibility.
		return apiservererrors.ServerError(apiservererrors.ErrPerm)
	} else if err != nil {
		return apiservererrors.ServerError(err)
	}

	nics, err := commonnetwork.ParamsNetworkConfigToDomain(ctx, args.Config, network.OriginMachine)
	if err != nil {
		return apiservererrors.ServerError(err)
	}

	if err := api.networkService.SetMachineNetConfig(ctx, mUUID, nics); err != nil {
		return apiservererrors.ServerError(err)
	}
	return nil
}

// SetStatus sets the status of the specified machine.
func (api *MachinerAPI) SetStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canModify, err := api.getCanModify(ctx)
	if err != nil {
		return results, err
	}
	now := api.clock.Now()
	for i, entity := range args.Entities {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canModify(machineTag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineName := machine.Name(machineTag.Id())

		err = api.statusService.SetMachineStatus(ctx, machineName, status.StatusInfo{
			Status:  status.Status(entity.Status),
			Message: entity.Info,
			Data:    entity.Data,
			Since:   &now,
		})
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}

// SetMachineAddresses is not supported in MachinerAPI at version 5.
// Deprecated: SetMachineAddresses is being deprecated.
func (api *MachinerAPI) SetMachineAddresses(ctx context.Context, args params.SetMachinesAddresses) (params.ErrorResults, error) {
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
	for i := range args.Entities {
		// 3.6 controller models can not be migrated, so we can always just
		// return the host-units job. The api-server job is not required.
		results.Results[i].Jobs = []string{"host-units"}
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
	canModify, err := api.getCanModify(ctx)
	if err != nil {
		return results, err
	}

	for i, entity := range args.Entities {
		err := api.recordAgentStartInformation(ctx, entity.Tag, "", canModify)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// RecordAgentStartInformation syncs the machine model with information
// reported by a machine agent when it starts.
func (api *MachinerAPI) RecordAgentStartInformation(ctx context.Context, args params.RecordAgentStartInformationArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	canModify, err := api.getCanModify(ctx)
	if err != nil {
		return results, err
	}

	for i, arg := range args.Args {
		err := api.recordAgentStartInformation(ctx, arg.Tag, arg.Hostname, canModify)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// Watch starts an NotifyWatcher for each given entity.
func (api *MachinerAPI) Watch(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}

	canRead, err := api.getCanRead(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canRead(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		switch tag := tag.(type) {
		case names.MachineTag:
			watcherID, err := api.watchMachine(ctx, machine.Name(tag.Id()))
			result.Results[i] = params.NotifyWatchResult{
				NotifyWatcherId: watcherID,
				Error:           apiservererrors.ServerError(err),
			}

		default:
			result.Results[i].Error = apiservererrors.ServerError(
				errors.NotImplementedf("agent type of %s", tag.Kind()),
			)
		}
	}
	return result, nil
}

func (api *MachinerAPI) watchMachine(ctx context.Context, machineName machine.Name) (string, error) {
	watch, err := api.machineService.WatchMachineLife(ctx, machineName)
	if err != nil {
		return "", err
	}
	id, _, err := internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, watch)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}

func (api *MachinerAPI) recordAgentStartInformation(ctx context.Context, tag, hostname string, authChecker common.AuthFunc) error {
	mTag, err := api.canModify(tag, authChecker)
	if err != nil {
		return err
	}

	mUUID, err := api.machineService.GetMachineUUID(ctx, machine.Name(mTag.Id()))
	if errors.Is(err, machineerrors.MachineNotFound) {
		return errors.NotFoundf("machine %q", mTag.Id())
	} else if err != nil {
		return err
	}

	err = api.machineService.SetMachineHostname(ctx, mUUID, hostname)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return errors.NotFoundf("machine %q", mTag.Id())
	} else if err != nil {
		return err
	}
	return nil
}

func (api *MachinerAPI) canModify(tag string, authChecker common.AuthFunc) (names.MachineTag, error) {
	mTag, err := names.ParseMachineTag(tag)
	if err != nil {
		return names.MachineTag{}, apiservererrors.ErrPerm
	} else if !authChecker(mTag) {
		return names.MachineTag{}, apiservererrors.ErrPerm
	}
	return mTag, nil
}
