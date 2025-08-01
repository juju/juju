// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/blockcommand"
	modelerrors "github.com/juju/juju/domain/model/errors"
	interrors "github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.apiserver.common")

// BlockCommandService defines methods for interacting with block commands.
type BlockCommandService interface {
	// GetBlockSwitchedOn returns the optional block message if it is switched
	// on for the given type.
	GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error)

	// GetBlocks returns all the blocks that are currently in place.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}

// DestroyController sets the controller model to Dying and, if requested,
// schedules cleanups so that all of the hosted models are destroyed, or
// otherwise returns an error indicating that there are hosted models
// remaining.
func DestroyController(
	ctx context.Context,
	modelUUIDs []model.UUID,
	blockCommandService BlockCommandService,
	modelInfoService ModelInfoService,
	modelService ModelService,
	blockCommandServiceGetter func(context.Context, model.UUID) (BlockCommandService, error),
	destroyHostedModels bool,
	destroyStorage *bool,
	force *bool,
	maxWait *time.Duration,
	modelTimeout *time.Duration,
) error {

	isControllerModel, err := modelInfoService.IsControllerModel(ctx)
	if err != nil {
		return interrors.Capture(err)

	}
	if !isControllerModel {
		return interrors.Errorf("current model is not the controller model")
	}

	if destroyHostedModels {
		for _, uuid := range modelUUIDs {
			svc, err := blockCommandServiceGetter(ctx, uuid)
			if err != nil {
				return interrors.Capture(err)
			}

			check := common.NewBlockChecker(svc)
			if err = check.DestroyAllowed(ctx); interrors.Is(err, modelerrors.NotFound) {
				logger.Errorf(ctx, "model %v not found, skipping", uuid)
				continue
			} else if err != nil {
				return interrors.Capture(err)
			}
		}
	}
	return checkForceForControllerModel(ctx, blockCommandService, modelInfoService, force)
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
