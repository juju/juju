// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"context"
	"sort"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coreapplication "github.com/juju/juju/core/application"
	corecontroller "github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/blockcommand"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/rpc/params"
)

// ControllerNodeService describes the maintenance of controller entries.
type ControllerNodeService interface {
	// GetControllerAPIAddresses returns the list of API addresses for all
	// controllers.
	GetControllerAPIAddresses(ctx context.Context) (map[string]network.HostPorts, error)
	// GetControllerIDs returns the list of controller IDs from the controller node
	// records.
	GetControllerIDs(ctx context.Context) ([]string, error)
	// CurateNodes modifies the known control plane by adding and removing
	// controller node records according to the input slices.
	CurateNodes(ctx context.Context, toAdd, toRemove []string) error
}

// ApplicationService instances add units to an application in dqlite state.
type ApplicationService interface {
	// AddControllerIAASUnits adds the specified controller units to the
	// controller IAAS application.
	AddControllerIAASUnits(ctx context.Context, controllerIDs []string, units []applicationservice.AddIAASUnitArg) ([]coremachine.Name, error)
}

// BlockCommandService defines methods for interacting with block commands.
type BlockCommandService interface {
	// GetBlockSwitchedOn returns the optional block message if it is switched
	// on for the given type.
	GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error)

	// GetBlocks returns all the blocks that are currently in place.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}

// HighAvailabilityAPI implements the HighAvailability interface and is the concrete
// implementation of the api end point.
type HighAvailabilityAPI struct {
	controllerTag         names.ControllerTag
	isControllerModel     bool
	controllerNodeService ControllerNodeService
	applicationService    ApplicationService
	blockCommandService   BlockCommandService
	authorizer            facade.Authorizer
	logger                corelogger.Logger
}

// HighAvailabilityAPIV2 implements v2 of the high availability facade.
type HighAvailabilityAPIV2 struct {
	HighAvailabilityAPI
}

// EnableHA adds controller machines as necessary to ensure the
// controller has the number of machines specified.
func (api *HighAvailabilityAPI) EnableHA(
	ctx context.Context, args params.ControllersSpecs,
) (params.ControllersChangeResults, error) {
	results := params.ControllersChangeResults{}

	err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, api.controllerTag)
	if err != nil {
		return results, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	if numSpecs := len(args.Specs); numSpecs == 0 {
		return results, nil
	} else if numSpecs > 1 {
		return results, errors.New("only one controller spec is supported")
	}

	result, err := api.enableHASingle(ctx, args.Specs[0])
	results.Results = make([]params.ControllersChangeResult, 1)
	results.Results[0].Result = result
	results.Results[0].Error = apiservererrors.ServerError(err)
	return results, nil
}

func (api *HighAvailabilityAPI) enableHASingle(ctx context.Context, spec params.ControllersSpec) (
	params.ControllersChanges, error,
) {
	if !api.isControllerModel {
		return params.ControllersChanges{}, errors.NotSupportedf("workload models")
	}
	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(api.blockCommandService)
	if err := blockChecker.ChangeAllowed(ctx); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	// We need to check that the number of controllers is not less than the
	// number of existing controllers.
	controllerIDs, err := api.controllerNodeService.GetControllerIDs(ctx)
	if err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	// The call to add controller units is purely additive, meaning that it will
	// never remove any existing controllers.
	required := spec.NumControllers
	if required == 0 {
		required = corecontroller.DefaultControllerCount - len(controllerIDs)
		if required <= 0 {
			// If there are no controllers required, we return an empty result.
			return params.ControllersChanges{}, nil
		}
	}

	args := make([]applicationservice.AddIAASUnitArg, 0, required)
	for i := 0; i < required; i++ {
		var placement *instance.Placement
		if i < len(spec.Placement) {
			placement, err = instance.ParsePlacement(spec.Placement[i])
			if err != nil {
				return params.ControllersChanges{}, errors.Trace(err)
			}
		}

		args = append(args, applicationservice.AddIAASUnitArg{
			AddUnitArg: applicationservice.AddUnitArg{
				Placement: placement,
			},
		})
	}

	machineNames, err := api.applicationService.AddControllerIAASUnits(ctx, controllerIDs, args)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.ControllersChanges{}, errors.NotFoundf("controller application %q", coreapplication.ControllerApplicationName)
	} else if err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	names := transform.Slice(machineNames, func(name coremachine.Name) string {
		return name.String()
	})
	if err := api.controllerNodeService.CurateNodes(ctx, names, nil); err != nil {
		// TODO (stickupkid): We're in a bit of jam here. We could try and
		// remove the units and machines, straight away, but we have to be
		// careful not to remove any machines that are already in use by
		// other applications/units.
		return params.ControllersChanges{}, errors.Trace(err)
	}

	// We don't support removal or converting an existing machine (hot standby)
	// of controllers in this API, so we return just the added and existing
	// controllers.
	return params.ControllersChanges{
		Added:      names,
		Maintained: controllerIDs,
	}, nil
}

// ControllerDetails is only available on V3 or later.
func (api *HighAvailabilityAPIV2) ControllerDetails(_ struct{}) {}

// ControllerDetails returns details about each controller node.
func (api *HighAvailabilityAPI) ControllerDetails(
	ctx context.Context,
) (params.ControllerDetailsResults, error) {
	results := params.ControllerDetailsResults{}

	err := api.authorizer.HasPermission(ctx, permission.LoginAccess, api.controllerTag)
	if err != nil {
		return results, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	hostPorts, err := api.controllerNodeService.GetControllerAPIAddresses(ctx)
	if errors.Is(err, controllernodeerrors.EmptyAPIAddresses) {
		// If there are no API addresses, we return an empty result.
		return results, nil
	} else if err != nil {
		return results, apiservererrors.ServerError(errors.Trace(err))
	}

	details := make([]params.ControllerDetails, 0, len(hostPorts))
	for id, hostPort := range hostPorts {
		details = append(details, params.ControllerDetails{
			ControllerId: id,
			APIAddresses: hostPort.FilterUnusable().Unique().Strings(),
		})
	}

	sort.Slice(details, func(i, j int) bool {
		return details[i].ControllerId < details[j].ControllerId
	})

	results.Results = details

	return results, nil
}
