// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// DestroyController will attempt to destroy the controller. If the args
// specify the removal of blocks or the destruction of the models, this
// method will attempt to do so.
func (s *ControllerAPI) DestroyController(args params.DestroyControllerArgs) error {
	controllerEnv, err := s.state.ControllerModel()
	if err != nil {
		return errors.Trace(err)
	}
	systemTag := controllerEnv.ModelTag()

	if err = s.ensureNotBlocked(args); err != nil {
		return errors.Trace(err)
	}

	// If we are destroying models, we need to tolerate living
	// models but set the controller to dying to prevent new
	// models sneaking in. If we are not destroying hosted models,
	// this will fail if any hosted models are found.
	if args.DestroyModels {
		return errors.Trace(common.DestroyModelIncludingHosted(s.state, systemTag))
	}
	if err = common.DestroyModel(s.state, systemTag); state.IsHasHostedModelsError(err) {
		err = errors.New("controller model cannot be destroyed before all other models are destroyed")
	}
	return errors.Trace(err)
}

func (s *ControllerAPI) ensureNotBlocked(args params.DestroyControllerArgs) error {
	// If there are blocks let the user know.
	blocks, err := s.state.AllBlocksForController()
	if err != nil {
		logger.Debugf("Unable to get blocks for controller: %s", err)
		return errors.Trace(err)
	}

	if len(blocks) > 0 {
		return common.OperationBlockedError("found blocks in controller models")
	}
	return nil
}
