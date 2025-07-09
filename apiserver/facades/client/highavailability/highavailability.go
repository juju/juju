// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/unit"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/blockcommand"
	"github.com/juju/juju/rpc/params"
)

// NodeService describes the maintenance of controller entries.
type NodeService interface {
	// CurateNodes modifies the control place by adding and
	// removing node entries based on the input slices.
	CurateNodes(context.Context, []string, []string) error
}

// ApplicationService instances add units to an application in dqlite state.
type ApplicationService interface {
	// AddIAASUnits adds units to the application with the given name.
	AddIAASUnits(
		ctx context.Context,
		name string,
		units ...applicationservice.AddIAASUnitArg,
	) ([]unit.Name, error)
}

// ControllerConfigService instances read the controller config.
type ControllerConfigService interface {
	ControllerConfig(ctx context.Context) (controller.Config, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
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
	nodeService             NodeService
	applicationService      ApplicationService
	controllerConfigService ControllerConfigService
	networkService          NetworkService
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

	return results, nil
}
