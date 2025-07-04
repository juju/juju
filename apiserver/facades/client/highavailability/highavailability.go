// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"context"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/blockcommand"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/rpc/params"
)

// ControllerNodeService describes the maintenance of controller entries.
type ControllerNodeService interface {
	// GetControllerAPIAddresses returns the list of API addresses for all
	// controllers.
	GetControllerAPIAddresses(ctx context.Context) (map[string]network.HostPorts, error)
}

// ApplicationService instances add units to an application in dqlite state.
type ApplicationService interface {
}

// ControllerConfigService instances read the controller config.
type ControllerConfigService interface {
	ControllerConfig(ctx context.Context) (controller.Config, error)
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
	controllerTag           names.ControllerTag
	isControllerModel       bool
	controllerNodeService   ControllerNodeService
	applicationService      ApplicationService
	controllerConfigService ControllerConfigService
	blockCommandService     BlockCommandService
	authorizer              facade.Authorizer
	logger                  corelogger.Logger
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

	if len(args.Specs) == 0 {
		return results, nil
	}
	if len(args.Specs) > 1 {
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
		return params.ControllersChanges{}, errors.New("unsupported with workload models")
	}
	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(api.blockCommandService)
	if err := blockChecker.ChangeAllowed(ctx); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	return params.ControllersChanges{}, nil
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
