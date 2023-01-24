// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/environs/config"
)

// DestroyConfirmationCommandBase provides common attributes and methods that
// commands require to confirm the execution of destroy-* commands
type DestroyConfirmationCommandBase struct {
	assumeYes      bool // DEPRECATED
	assumeNoPrompt bool
}

// SetFlags implements Command.SetFlags.
func (c *DestroyConfirmationCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.assumeYes, "y", false, "Do not ask for confirmation. Option present for backwards compatibility with Juju 2.9")
	f.BoolVar(&c.assumeYes, "yes", false, "")
	f.BoolVar(&c.assumeNoPrompt, "no-prompt", false, "Do not ask for confirmation")
}

// Run implements Command.Run
func (c *DestroyConfirmationCommandBase) Run(ctx *cmd.Context) error {
	if c.assumeYes {
		ctx.Warningf("'-y'/'--yes' flags are deprecated and will be removed in Juju 3.2\n")
	}
	return nil
}

// NeedsConfirmation returns indicates whether confirmation is required or not.
func (c *DestroyConfirmationCommandBase) NeedsConfirmation() bool {
	return !(c.assumeYes || c.assumeNoPrompt)
}

type ModelConfigAPI interface {
	ModelGet() (map[string]interface{}, error)
}

// RemoveConfirmationCommandBase provides common attributes and methods that
// commands require to confirm the execution of remove-* commands
type RemoveConfirmationCommandBase struct {
	assumeNoPrompt bool
}

// SetFlags implements Command.SetFlags.
func (c *RemoveConfirmationCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.assumeNoPrompt, "no-prompt", false, "Do not ask for confirmation")
}

// NeedsConfirmation returns indicates whether confirmation is required or not.
func (c *RemoveConfirmationCommandBase) NeedsConfirmation(client ModelConfigAPI) bool {
	if c.assumeNoPrompt {
		return false
	}

	configAttrs, err := client.ModelGet()
	if err != nil {
		// Play it safe
		return true
	}
	cfg, err := config.New(config.UseDefaults, configAttrs)
	if err != nil {
		return true
	}
	modes, _ := cfg.Mode()
	return modes.Contains(config.RequiresPromptsMode)
}
