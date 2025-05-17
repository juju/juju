// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
)

// DestroyController destroys the controller.
//
// If the args specify the destruction of the models, this method will
// attempt to do so. Otherwise, if the controller has any non-empty,
// non-Dead hosted models, then an error with the code
// params.CodeHasHostedModels will be transmitted.
func (c *ControllerAPI) DestroyController(ctx context.Context, args params.DestroyControllerArgs) error {
	err := c.authorizer.HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(c.controllerUUID))
	if err != nil {
		return errors.Trace(err)
	}

	modelUUIDs, err := c.modelService.ListModelUUIDs(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if err := ensureNotBlocked(ctx, c.modelService, c.blockCommandServiceGetter, modelUUIDs, c.logger); err != nil {
		return errors.Trace(err)
	}

	stModel, err := c.state.Model()
	if err != nil {
		return errors.Trace(err)
	}

	// If we are destroying models, we need to tolerate living
	// models but set the controller to dying to prevent new
	// models sneaking in. If we are not destroying hosted models,
	// this will fail if any hosted models are found.
	backend := commonmodel.NewModelManagerBackend(stModel, c.statePool)
	err = commonmodel.DestroyController(
		ctx,
		c.controllerModelUUID,
		modelUUIDs,
		backend,
		c.blockCommandService,
		c.modelInfoService,
		c.modelService,
		func(ctx context.Context, u model.UUID) (commonmodel.StatusService, error) {
			return c.statusServiceGetter(ctx, u)
		},
		func(ctx context.Context, u model.UUID) (commonmodel.BlockCommandService, error) {
			return c.blockCommandServiceGetter(ctx, u)
		},
		args.DestroyModels, args.DestroyStorage,
		args.Force, args.MaxWait, args.ModelTimeout,
	)

	if err != nil {
		c.logger.Warningf(ctx, "failed destroying controller: %v", err)
		return errors.Trace(err)
	}

	return nil
}

func ensureNotBlocked(
	ctx context.Context,
	modelService ModelService,
	blockCommandServiceGetter func(context.Context, model.UUID) (BlockCommandService, error),
	uuids []model.UUID,
	logger corelogger.Logger,
) error {

	// If there are blocks let the user know.
	for _, uuid := range uuids {
		blockService, err := blockCommandServiceGetter(ctx, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		blocks, err := blockService.GetBlocks(ctx)
		if err != nil {
			logger.Debugf(ctx, "unable to get blocks for controller: %s", err)
			return errors.Trace(err)
		}

		if len(blocks) > 0 {
			return apiservererrors.OperationBlockedError("found blocks in controller models")
		}
	}
	return nil
}
