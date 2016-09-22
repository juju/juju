// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Block", 2, NewAPI)
}

// Block defines the methods on the block API end point.
type Block interface {
	// List returns all current blocks for this environment.
	List() (params.BlockResults, error)

	// SwitchBlockOn switches desired block type on for this
	// environment.
	SwitchBlockOn(params.BlockSwitchParams) params.ErrorResult

	// SwitchBlockOff switches desired block type off for this
	// environment.
	SwitchBlockOff(params.BlockSwitchParams) params.ErrorResult
}

// API implements Block interface and is the concrete
// implementation of the api end point.
type API struct {
	access     blockAccess
	authorizer facade.Authorizer
}

// NewAPI returns a new block API facade.
func NewAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {

	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		access:     getState(st),
		authorizer: authorizer,
	}, nil
}

var getState = func(st *state.State) blockAccess {
	return stateShim{st}
}

func (a *API) checkCanRead() error {
	canRead, err := a.authorizer.HasPermission(permission.ReadAccess, a.access.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if !canRead {
		return common.ErrPerm
	}
	return nil
}

func (a *API) checkCanWrite() error {
	canWrite, err := a.authorizer.HasPermission(permission.WriteAccess, a.access.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if !canWrite {
		return common.ErrPerm
	}
	return nil
}

// List implements Block.List().
func (a *API) List() (params.BlockResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.BlockResults{}, err
	}

	all, err := a.access.AllBlocks()
	if err != nil {
		return params.BlockResults{}, common.ServerError(err)
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
		result.Error = common.ServerError(err)
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
func (a *API) SwitchBlockOn(args params.BlockSwitchParams) params.ErrorResult {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}
	}

	err := a.access.SwitchBlockOn(state.ParseBlockType(args.Type), args.Message)
	return params.ErrorResult{Error: common.ServerError(err)}
}

// SwitchBlockOff implements Block.SwitchBlockOff().
func (a *API) SwitchBlockOff(args params.BlockSwitchParams) params.ErrorResult {
	if err := a.checkCanWrite(); err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}
	}

	err := a.access.SwitchBlockOff(state.ParseBlockType(args.Type))
	return params.ErrorResult{Error: common.ServerError(err)}
}
