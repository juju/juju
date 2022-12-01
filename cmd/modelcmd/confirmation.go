// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// ConfirmationCommandBase provides common attributes and methods that
// commands require to confirm the execution.
type ConfirmationCommandBase struct {
	assumeNoPrompt bool
}

// SetFlags implements Command.SetFlags.
func (c *ConfirmationCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.assumeNoPrompt, "no-prompt", false, "Do not ask for confirmation")
}

// Init implements Command.Init.
func (c *ConfirmationCommandBase) Init(args []string) error {
	if !c.assumeNoPrompt {
		assumeNoPrompt, skipErr := jujucmd.CheckSkipConfirmationEnvVar()
		if skipErr != nil && !errors.Is(skipErr, errors.NotFound) {
			return errors.Trace(skipErr)
		}
		if !errors.Is(skipErr, errors.NotFound) {
			c.assumeNoPrompt = assumeNoPrompt
		}
	}
	return nil
}

// NeedsConfirmation returns if flags require the confirmation or not.
func (c *ConfirmationCommandBase) NeedsConfirmation() bool {
	return !c.assumeNoPrompt
}
