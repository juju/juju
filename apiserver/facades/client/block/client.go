// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// Block defines the methods on the block API end point.
type Block interface {
	// List returns all current blocks for this model.
	List() (params.BlockResults, error)

	// SwitchBlockOn switches desired block type on for this
	// model.
	SwitchBlockOn(params.BlockSwitchParams) params.ErrorResult

	// SwitchBlockOff switches desired block type off for this
	// model.
	SwitchBlockOff(params.BlockSwitchParams) params.ErrorResult
}

// API implements Block interface and is the concrete
// implementation of the api end point.
type API struct {
	access     blockAccess
	authorizer facade.Authorizer
}

var getState = func(st *state.State, m *state.Model) blockAccess {
	return stateShim{st, m}
}

func (a *API) checkCanRead() error {
	err := a.authorizer.HasPermission(permission.ReadAccess, a.access.ModelTag())
	return err
}

func (a *API) checkCanWrite() error {
	err := a.authorizer.HasPermission(permission.WriteAccess, a.access.ModelTag())
	return err
}

// List implements Block.List().
func (a *API) List(ctx context.Context) (params.BlockResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.BlockResults{}, err
	}

	all, err := a.access.AllBlocks()
	if err != nil {
		return params.BlockResults{}, apiservererrors.ServerError(err)
	}
	found := make([]params.BlockResult, len(all))
	for i, one := range all {
		found[i] = convertBlock(one)
	}
	return params.BlockResults{Results: found}, nil
}

func convertBlock(b state.Block) params.BlockResult {
	result := params.BlockResult{}
	tag, err := b.Tag()
	if err != nil {
		err := errors.Annotatef(err, "getting block %v", b.Type().String())
		result.Error = apiservererrors.ServerError(err)
	}
	result.Result = params.Block{
		Id:      b.Id(),
		Tag:     tag.String(),
		Type:    b.Type().String(),
		Message: b.Message(),
	}
	return result
}

// SwitchBlockOn implements Block.SwitchBlockOn().
func (a *API) SwitchBlockOn(ctx context.Context, args params.BlockSwitchParams) params.ErrorResult {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}

	err := a.access.SwitchBlockOn(state.ParseBlockType(args.Type), args.Message)
	return params.ErrorResult{Error: apiservererrors.ServerError(err)}
}

// SwitchBlockOff implements Block.SwitchBlockOff().
func (a *API) SwitchBlockOff(ctx context.Context, args params.BlockSwitchParams) params.ErrorResult {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}

	err := a.access.SwitchBlockOff(state.ParseBlockType(args.Type))
	return params.ErrorResult{Error: apiservererrors.ServerError(err)}
}
