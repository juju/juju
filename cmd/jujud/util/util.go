// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"context"

	"github.com/juju/errors"

	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/core/logger"
	jworker "github.com/juju/juju/internal/worker"
)

// AgentDone processes the error returned by an exiting agent.
func AgentDone(logger logger.Logger, err error) error {
	switch {
	case errors.Is(err, jworker.ErrTerminateAgent),
		errors.Is(err, jworker.ErrRebootMachine),
		errors.Is(err, jworker.ErrShutdownMachine):
		// These errors are swallowed here because we want to exit
		// the agent process without error, to avoid the init system
		// restarting us.
		err = nil
	}

	var ug *agenterrors.UpgradeReadyError
	if errors.As(err, &ug) {
		if err := ug.ChangeAgentTools(logger); err != nil {
			// Return and let the init system deal with the restart.
			err = errors.Annotate(err, "cannot change agent binaries")
			logger.Infof(context.TODO(), err.Error())
			return err
		}
	}
	if errors.Is(err, jworker.ErrRestartAgent) {
		logger.Warningf(context.TODO(), "agent restarting")
	}
	return err
}
