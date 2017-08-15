// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

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
	controllerModelUUID := args.statePool.SystemState().ModelUUID()
	if args.modelUUID == "" {
		// We allow the modelUUID to be empty so that:
		//    TODO: server a limited API at the root (empty modelUUID)
		//    just the user manager and model manager are able to accept
		//    requests that don't require a modelUUID, like add-model.
		if args.strict {
			return "", errors.Trace(common.UnknownModelError(args.modelUUID))
		}
		return controllerModelUUID, nil
	}
	if args.modelUUID == controllerModelUUID {
		return args.modelUUID, nil
	}
	if args.controllerModelOnly {
		return "", errors.Unauthorizedf("requested model %q is not the controller model", args.modelUUID)
	}
	if !names.IsValidModel(args.modelUUID) {
		return "", errors.Trace(common.UnknownModelError(args.modelUUID))
	}

	_, release, err := args.statePool.GetModel(args.modelUUID)
	if err != nil {
		return "", errors.Wrap(err, common.UnknownModelError(args.modelUUID))
	}
	release()
	return args.modelUUID, nil
}
