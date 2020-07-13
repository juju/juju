// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

// DestroyController destroys the controller.
//
// The v3 implementation of DestroyController ignores the DestroyStorage
// field of the arguments, and unconditionally destroys all storage in
// the controller.
//
// See ControllerAPIv4.DestroyController for more details.
func (c *ControllerAPIv3) DestroyController(args params.DestroyControllerArgs) error {
	if args.DestroyStorage != nil {
		return errors.New("destroy-storage unexpected on the v3 API")
	}
	destroyStorage := true
	args.DestroyStorage = &destroyStorage
	return destroyController(c.state, c.statePool, c.authorizer, args)
}

// DestroyController destroys the controller.
//
// If the args specify the destruction of the models, this method will
// attempt to do so. Otherwise, if the controller has any non-empty,
// non-Dead hosted models, then an error with the code
// params.CodeHasHostedModels will be transmitted.
func (c *ControllerAPI) DestroyController(args params.DestroyControllerArgs) error {
	return destroyController(c.state, c.statePool, c.authorizer, args)
}

func destroyController(
	st *state.State,
	pool *state.StatePool,
	authorizer facade.Authorizer,
	args params.DestroyControllerArgs,
) error {
	hasPermission, err := authorizer.HasPermission(permission.SuperuserAccess, st.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !hasPermission {
		return errors.Trace(apiservererrors.ErrPerm)
	}
	if err := ensureNotBlocked(st); err != nil {
		return errors.Trace(err)
	}

	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	// If we are destroying models, we need to tolerate living
	// models but set the controller to dying to prevent new
	// models sneaking in. If we are not destroying hosted models,
	// this will fail if any hosted models are found.
	backend := common.NewModelManagerBackend(model, pool)
	return errors.Trace(common.DestroyController(
		backend, args.DestroyModels, args.DestroyStorage,
	))
}

func ensureNotBlocked(st *state.State) error {
	// If there are blocks let the user know.
	blocks, err := st.AllBlocksForController()
	if err != nil {
		logger.Debugf("Unable to get blocks for controller: %s", err)
		return errors.Trace(err)
	}
	if len(blocks) > 0 {
		return apiservererrors.OperationBlockedError("found blocks in controller models")
	}
	return nil
}
