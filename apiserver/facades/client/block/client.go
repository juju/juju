// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/blockcommand"
	"github.com/juju/juju/rpc/params"
)

// BlockCommandService defines the methods that the BlockCommandService
// facade requires from the domain service.
type BlockCommandService interface {
	// SwitchBlockOn switches on a command block for a given type and message.
	SwitchBlockOn(ctx context.Context, t blockcommand.BlockType, message string) error
	// SwitchBlockOff disables block of specified type for the current model.
	SwitchBlockOff(ctx context.Context, t blockcommand.BlockType) error
	// GetBlocks returns all the blocks for the current model.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}

// Authorizer defines the methods that the BlockCommandService
type Authorizer interface {

	// HasPermission reports whether the given access is allowed for the given
	// target by the authenticated entity.
	HasPermission(ctx context.Context, operation permission.Access, target names.Tag) error
}

// API implements Block interface and is the concrete
// implementation of the api end point.
type API struct {
	modelTag   names.ModelTag
	service    BlockCommandService
	authorizer Authorizer
}

func (a *API) checkCanRead(ctx context.Context) error {
	err := a.authorizer.HasPermission(ctx, permission.ReadAccess, a.modelTag)
	return err
}

func (a *API) checkCanWrite(ctx context.Context) error {
	err := a.authorizer.HasPermission(ctx, permission.WriteAccess, a.modelTag)
	return err
}

// List implements Block.List().
func (a *API) List(ctx context.Context) (params.BlockResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.BlockResults{}, err
	}

	all, err := a.service.GetBlocks(ctx)
	if err != nil {
		return params.BlockResults{}, apiservererrors.ServerError(err)
	}
	found := make([]params.BlockResult, len(all))
	for i, one := range all {
		found[i] = convertBlock(a.modelTag, one)
	}
	return params.BlockResults{Results: found}, nil
}

func convertBlock(modelTag names.ModelTag, b blockcommand.Block) params.BlockResult {
	result := params.BlockResult{}
	result.Result = params.Block{
		Id:      b.UUID,
		Tag:     modelTag.String(),
		Type:    b.Type.String(),
		Message: b.Message,
	}
	return result
}

// SwitchBlockOn implements Block.SwitchBlockOn().
func (a *API) SwitchBlockOn(ctx context.Context, args params.BlockSwitchParams) params.ErrorResult {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}

	blockType, err := parseBlockType(args.Type)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}

	err = a.service.SwitchBlockOn(ctx, blockType, args.Message)
	return params.ErrorResult{Error: apiservererrors.ServerError(err)}
}

// SwitchBlockOff implements Block.SwitchBlockOff().
func (a *API) SwitchBlockOff(ctx context.Context, args params.BlockSwitchParams) params.ErrorResult {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}

	blockType, err := parseBlockType(args.Type)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}

	err = a.service.SwitchBlockOff(ctx, blockType)
	return params.ErrorResult{Error: apiservererrors.ServerError(err)}
}

func parseBlockType(str string) (blockcommand.BlockType, error) {
	switch str {
	case model.BlockDestroy:
		return blockcommand.DestroyBlock, nil
	case model.BlockRemove:
		return blockcommand.RemoveBlock, nil
	case model.BlockChange:
		return blockcommand.ChangeBlock, nil
	default:
		return -1, errors.NotValidf("unknown block type %q", str)
	}
}
