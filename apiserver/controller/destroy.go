// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

// DestroyController will attempt to destroy the controller. If the args
// specify the removal of blocks or the destruction of the models, this
// method will attempt to do so.
//
// If the controller has any non-Dead hosted models, then an error with
// the code params.CodeHasHostedModels will be transmitted, regardless of
// the value of the DestroyModels parameter. This is to inform the client
// that it should wait for hosted models to be completely cleaned up
// before proceeding.
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
	if err := common.DestroyModel(s.state, systemTag); err != nil {
		return errors.Trace(err)
	}
	return nil
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
