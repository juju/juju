// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/factory"
	"github.com/juju/juju/instance"
)

var logger = loggo.GetLogger("juju.cmd.jujud.reboot")

type RebootContext interface {
	Clock() clock.Clock
	Exec() utils.CommandRunner
}

func validateContext(c RebootContext) error {
	if c.Clock() == nil {
		return errors.NotValidf("Clock")
	} else if c.Exec() == nil {
		return errors.NotValidf("Exec")
	}
	return nil
}

type RebootAPI interface {
	Open() error
	ClearReboot() error
	RequestReboot() error
	Close() error
}

func ExecuteRebootOrShutdown(
	ctx RebootContext,
	rebootAPI RebootAPI,
	agentConfig container.ContainerAgentConfig,
	action params.RebootAction,
	timeout time.Duration,
) (retErr error) {

	if err := validateContext(ctx); err != nil {
		return errors.Trace(err)
	}

	newContainerManager := func(
		t instance.ContainerType,
		c container.ManagerConfig,
	) (container.ContainerManager, error) {
		return factory.NewContainerManager(t, c, nil)
	}
	containerTeardownWorker, err := container.WaitForContainerTeardownWorker(
		ctx.Clock(),
		newContainerManager,
		agentConfig,
	)
	if err != nil {
		return errors.Trace(err)
	}
	go func() {
		time.Sleep(timeout)
		containerTeardownWorker.Kill()
	}()
	if err := containerTeardownWorker.Wait(); err != nil {
		return errors.Trace(err)
	}

	// At this stage, all API connections would have been closed We
	// need to reopen the API to clear the reboot flag after
	// scheduling the reboot. It may be cleaner to do this in the
	// reboot worker, before returning the ErrRebootMachine.
	if err := rebootAPI.Open(); err != nil {
		return errors.Annotate(err, "cannot connect to state")
	}
	defer func() { retErr = rebootAPI.Close() }()

	logger.Infof("Clearing reboot request from state.")
	if err := rebootAPI.ClearReboot(); err != nil {
		return errors.Annotate(err, "cannot clear reboot request from state")
	}

	logger.Infof("Reboot: Executing reboot")
	const rebootDelay = 15 * time.Second
	if err := ExecuteReboot(ctx.Exec(), rebootDelay, action); err != nil {
		logger.Warningf("Cannot schedule a %q; reinstating reboot request.")
		if err := rebootAPI.RequestReboot(); err != nil {
			logger.Criticalf("Cannot reinstate reboot request after failing to schedule a reboot")
		}
		return errors.Annotate(err, "cannot reboot machine")
	}

	return nil
}

// ExecuteReboot will wait for all running containers to stop, and
// then execute a shutdown or a reboot (based on the action param)
func ExecuteReboot(exec utils.CommandRunner, delay time.Duration, action params.RebootAction) error {
	if action == params.ShouldDoNothing {
		return nil
	}

	cmd, args := buildRebootCommand(action, delay)
	if _, err := exec(cmd, args...); err != nil {
		return errors.Annotate(err, "cannot schedule reboot")
	}
	return nil
}
