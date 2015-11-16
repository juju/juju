// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

type DestroyEnvironmentCommand struct {
	*destroyEnvironmentCommand
}

func (c *DestroyEnvironmentCommand) AssumeYes() bool {
	return c.assumeYes
}

func NewDestroyEnvironmentCommand(apierr error) (cmd.Command, *DestroyEnvironmentCommand) {
	command := &destroyEnvironmentCommand{apierr: apierr}
	return envcmd.Wrap(
		command,
		envcmd.EnvSkipDefault,
		envcmd.EnvAllowEmpty,
	), &DestroyEnvironmentCommand{command}
}
