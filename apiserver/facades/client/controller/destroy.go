// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	modelerrors "github.com/juju/juju/domain/model/errors"
	interrors "github.com/juju/juju/internal/errors"
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

	isControllerModel, err := c.modelInfoService.IsControllerModel(ctx)
	if err != nil {
		return interrors.Capture(err)

	}
	if !isControllerModel {
		return interrors.Errorf("current model is not the controller model")
	}

	modelUUIDs, err := c.modelService.ListModelUUIDs(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := ensureNotBlocked(ctx, c.modelService, c.blockCommandServiceGetter, modelUUIDs, c.logger); err != nil {
		return errors.Trace(err)
	}

	// If we are destroying models, we need to tolerate living
	// models but set the controller to dying to prevent new
	// models sneaking in. If we are not destroying hosted models,
	// this will fail if any hosted models are found.
	if args.DestroyModels && len(modelUUIDs) != 0 {
		return interrors.Errorf("cannot destroy controller with hosted models, use --de")
	}

	for _, uuid := range modelUUIDs {
		svc, err := c.blockCommandServiceGetter(ctx, uuid)
		if err != nil {
			return interrors.Capture(err)
		}

		check := common.NewBlockChecker(svc)
		if err = check.DestroyAllowed(ctx); interrors.Is(err, modelerrors.NotFound) {
			continue
		} else if err != nil {
			return interrors.Capture(err)
		}
	}

	if err := checkForceForControllerModel(ctx, c.blockCommandService, c.modelInfoService, args.Force); err != nil {
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

func checkForceForControllerModel(
	ctx context.Context,
	blockCommandService BlockCommandService,
	modelInfoService ModelInfoService,
	force *bool,
) error {
	check := common.NewBlockChecker(blockCommandService)
	if err := check.DestroyAllowed(ctx); err != nil {
		return interrors.Capture(err)
	}

	notForcing := force == nil || !*force
	if notForcing {
		hasValidCredential, err := modelInfoService.HasValidCredential(ctx)

		if err != nil {
			return interrors.Capture(err)
		}
		if !hasValidCredential {
			return interrors.Errorf("invalid cloud credential, use --force")
		}
	}

	return nil
}
