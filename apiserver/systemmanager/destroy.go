// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// DestroySystem will attempt to destroy the system. If the args specify the
// removal of blocks or the destruction of the environments, this method will
// attempt to do so.
func (s *SystemManagerAPI) DestroySystem(args params.DestroySystemArgs) error {
	systemEnv, err := s.state.StateServerEnvironment()
	if err != nil {
		return errors.Trace(err)
	}
	systemTag := systemEnv.EnvironTag()

	if err = s.ensureNotBlocked(args); err != nil {
		return errors.Trace(err)
	}

	// If we are destroying environments, we need to tolerate living
	// environments but set the system to dying to prevent new environments
	// sneaking in. If we are not destroying hosted environments, this will
	// fail if any hosted environments are found.
	if args.DestroyEnvironments {
		return errors.Trace(common.DestroyEnvironmentIncludingHosted(s.state, systemTag))
	}
	if err = common.DestroyEnvironment(s.state, systemTag); state.IsHasHostedEnvironsError(err) {
		err = errors.New("state server environment cannot be destroyed before all other environments are destroyed")
	}
	return errors.Trace(err)
}

func (s *SystemManagerAPI) ensureNotBlocked(args params.DestroySystemArgs) error {
	// If there are blocks, and we aren't being told to ignore them, let the
	// user know.
	blocks, err := s.state.AllBlocksForSystem()
	if err != nil {
		logger.Debugf("Unable to get blocks for system: %s", err)
		return errors.Trace(err)
	}

	if len(blocks) > 0 {
		return common.OperationBlockedError("found blocks in system environments")
	}
	return nil
}
