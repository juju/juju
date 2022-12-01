// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// ConfirmationCommandBase provides common attributes and methods that
// commands require to confirm the execution.
type ConfirmationCommandBase struct {
	assumeYes      bool // DEPRECATED
	assumeNoPrompt bool
}

// SetFlags implements Command.SetFlags.
func (c *ConfirmationCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.assumeYes, "y", false, "Do not ask for confirmation. Option present for backwards compatibility with Juju 2.9")
	f.BoolVar(&c.assumeYes, "yes", false, "")
	f.BoolVar(&c.assumeNoPrompt, "no-prompt", false, "Do not ask for confirmation")
}

// Init implements Command.Init.
func (c *ConfirmationCommandBase) Init(args []string) error {
	if !c.assumeNoPrompt {
		assumeNoPrompt, skipErr := jujucmd.CheckSkipConfirmationEnvVar()
		if skipErr != nil && !errors.IsNotFound(skipErr) {
			return errors.Trace(skipErr)
		}
		if !errors.IsNotFound(skipErr) {
			c.assumeNoPrompt = assumeNoPrompt
		}
	}
	return nil
}

// Run implements Command.Run
func (c *ConfirmationCommandBase) Run(ctx *cmd.Context) error {
	if c.assumeYes {
		fmt.Fprint(ctx.Stdout, "WARNING: '-y'/'--yes' flags are deprecated and will be removed in Juju 3.1\n")
	}
	return nil
}

// NeedsConfirmation returns if flags require the confirmation or not.
func (c *ConfirmationCommandBase) NeedsConfirmation() bool {
	return !(c.assumeYes || c.assumeNoPrompt)
}
