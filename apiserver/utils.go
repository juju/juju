// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// isMachineWithJob returns whether the given entity is a machine that
// is configured to run the given job.
func isMachineWithJob(e state.Entity, j state.MachineJob) bool {
	m, ok := e.(*state.Machine)
	if !ok {
		return false
	}
	for _, mj := range m.Jobs() {
		if mj == j {
			return true
		}
	}
	return false
}

func setPassword(e state.Authenticator, password string) error {
	// Catch expected common case of misspelled
	// or missing Password parameter.
	if password == "" {
		return errors.New("password is empty")
	}
	return e.SetPassword(password)
}

type validateArgs struct {
	statePool *state.StatePool
	modelUUID string
	// strict validation does not allow empty UUID values
	strict bool
	// controllerModelOnly only validates the controller model
	controllerModelOnly bool
}

// validateModelUUID is the common validator for the various
// apiserver components that need to check for a valid model
// UUID.  An empty modelUUID means that the connection has come in at
// the root of the URL space and refers to the controller
// model.
//
// It returns the validated model UUID.
func validateModelUUID(args validateArgs) (string, error) {
	ssState := args.statePool.SystemState()
	if args.modelUUID == "" {
		// We allow the modelUUID to be empty for 2 cases
		// 1) Compatibility with older clients
		// 2) TODO: server a limited API at the root (empty modelUUID)
		//    with just the user manager and model manager
		//    if the connection comes over a sufficiently up to date
		//    login command.
		if args.strict {
			return "", errors.Trace(common.UnknownModelError(args.modelUUID))
		}
		logger.Debugf("validate model uuid: empty modelUUID")
		return ssState.ModelUUID(), nil
	}
	if args.modelUUID == ssState.ModelUUID() {
		logger.Debugf("validate model uuid: controller model - %s", args.modelUUID)
		return args.modelUUID, nil
	}
	if args.controllerModelOnly {
		return "", errors.Unauthorizedf("requested model %q is not the controller model", args.modelUUID)
	}
	if !names.IsValidModel(args.modelUUID) {
		return "", errors.Trace(common.UnknownModelError(args.modelUUID))
	}
	modelTag := names.NewModelTag(args.modelUUID)
	if _, err := ssState.GetModel(modelTag); err != nil {
		return "", errors.Wrap(err, common.UnknownModelError(args.modelUUID))
	}
	logger.Debugf("validate model uuid: %s", args.modelUUID)
	return args.modelUUID, nil
}
