// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/keyvalues"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/quota"
)

// StateSetCommand implements the state-set command.
type StateSetCommand struct {
	cmd.CommandBase
	ctx          Context
	StateValues  map[string]string
	keyValueFile cmd.FileVar
}

// NewStateSetCommand returns a state-set command.
func NewStateSetCommand(ctx Context) (cmd.Command, error) {
	return &StateSetCommand{ctx: ctx}, nil
}

// Info returns information about the Command.
// Info implements part of the cmd.Command interface.
func (c *StateSetCommand) Info() *cmd.Info {
	doc := `
state-set sets the value of the server side state specified by key.

The --file option should be used when one or more key-value pairs
are too long to fit within the command length limit of the shell
or operating system. The file will contain a YAML map containing
the settings as strings.  Settings in the file will be overridden
by any duplicate key-value arguments. A value of "-" for the filename
means <stdin>.

The following fixed size limits apply:
- Length of stored keys cannot exceed %d bytes.
- Length of stored values cannot exceed %d bytes.

See also:
    state-delete
    state-get
`
	return jujucmd.Info(&cmd.Info{
		Name:    "state-set",
		Args:    "key=value [key=value ...]",
		Purpose: "set server-side-state values",
		Doc: fmt.Sprintf(
			doc,
			quota.MaxCharmStateKeySize,
			quota.MaxCharmStateValueSize,
		),
	})
}

// SetFlags adds command specific flags to the flag set.
// SetFlags implements part of the cmd.Command interface.
func (c *StateSetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.keyValueFile.SetStdin()
	f.Var(&c.keyValueFile, "file", "file containing key-value pairs")
}

// Init initializes the Command before running.
// Init implements part of the cmd.Command interface.
func (c *StateSetCommand) Init(args []string) error {
	if args == nil {
		return nil
	}

	// The overrides will be applied during Run when c.keyValueFile is handled.
	overrides, err := keyvalues.Parse(args, true)
	if err != nil {
		return errors.Trace(err)
	}
	c.StateValues = overrides
	return nil
}

// Run will execute the Command as directed by the options and positional
// arguments passed to Init.
// Run implements part of the cmd.Command interface.
func (c *StateSetCommand) Run(ctx *cmd.Context) error {
	if err := c.handleKeyValueFile(ctx); err != nil {
		return errors.Trace(err)
	}

	for k, v := range c.StateValues {
		if err := c.ctx.SetCharmStateValue(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (c *StateSetCommand) handleKeyValueFile(ctx *cmd.Context) error {
	if c.keyValueFile.Path == "" {
		return nil
	}

	file, err := c.keyValueFile.Open(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = file.Close() }()

	kvs, err := readSettings(file)
	if err != nil {
		return errors.Trace(err)
	}

	overrides := c.StateValues
	for k, v := range overrides {
		kvs[k] = v
	}
	c.StateValues = kvs
	return nil
}
