// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
)

// DestroyController destroys the controller.
//
// If the args specify the destruction of the models, this method will
// attempt to do so. Otherwise, if the controller has any non-empty,
// non-Dead hosted models, then an error with the code
// params.CodeHasHostedModels will be transmitted.
func (c *ControllerAPI) DestroyController(ctx context.Context, args params.DestroyControllerArgs) error {
	err := c.authorizer.HasPermission(permission.SuperuserAccess, c.controllerTag)
	if err != nil {
		return errors.Trace(err)
	}

	if err := ensureNotBlocked(c.state, c.logger); err != nil {
		return errors.Trace(err)
	}

	model, err := c.state.Model()
	if err != nil {
		return errors.Trace(err)
	}

	// If we are destroying models, we need to tolerate living
	// models but set the controller to dying to prevent new
	// models sneaking in. If we are not destroying hosted models,
	// this will fail if any hosted models are found.
	backend := common.NewModelManagerBackend(environs.ProviderConfigSchemaSource(c.cloudService), model, c.statePool)
	return errors.Trace(common.DestroyController(
		ctx,
		backend, args.DestroyModels, args.DestroyStorage,
		args.Force, args.MaxWait, args.ModelTimeout,
	))
}

func ensureNotBlocked(st Backend, logger corelogger.Logger) error {
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
