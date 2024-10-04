// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

const (
	// RebootSkip is a noop.
	RebootSkip RebootPriority = iota
	// RebootAfterHook means wait for current hook to finish before
	// rebooting.
	RebootAfterHook
	// RebootNow means reboot immediately, killing and requeueing the
	// calling hook
	RebootNow
)

// JujuRebootCommand implements the juju-reboot command.
type JujuRebootCommand struct {
	cmd.CommandBase
	ctx Context
	Now bool
}

func NewJujuRebootCommand(ctx Context) (cmd.Command, error) {
	return &JujuRebootCommand{ctx: ctx}, nil
}

const rebootDoc = `
juju-reboot causes the host machine to reboot, after stopping all containers
hosted on the machine.

An invocation without arguments will allow the current hook to complete, and
will only cause a reboot if the hook completes successfully.

If the --now flag is passed, the current hook will terminate immediately, and
be restarted from scratch after reboot. This allows charm authors to write
hooks that need to reboot more than once in the course of installing software.

The --now flag cannot terminate a debug-hooks session; hooks using --now should
be sure to terminate on unexpected errors, so as to guarantee expected behaviour
in all situations.

juju-reboot is not supported when running actions.
`

const rebootExamples = `
    # immediately reboot
    juju-reboot --now

    # Reboot after current hook exits
    juju-reboot
`

func (c *JujuRebootCommand) Info() *cmd.Info {

	return jujucmd.Info(&cmd.Info{
		Name:     "juju-reboot",
		Args:     "",
		Purpose:  "Reboot the host machine.",
		Doc:      rebootDoc,
		Examples: rebootExamples,
	})
}

func (c *JujuRebootCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Now, "now", false, "reboot immediately, killing the invoking process")
}

func (c *JujuRebootCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *JujuRebootCommand) Run(ctx *cmd.Context) error {
	if _, err := c.ctx.ActionParams(); err == nil {
		return errors.New("juju-reboot is not supported when running an action.")
	}

	rebootPriority := RebootAfterHook
	if c.Now {
		rebootPriority = RebootNow
	}

	return c.ctx.RequestReboot(rebootPriority)
}
