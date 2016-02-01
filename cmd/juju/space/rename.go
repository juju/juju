// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

func newRenameCommand() cmd.Command {
	return modelcmd.Wrap(&renameCommand{})
}

// renameCommand calls the API to rename an existing network space.
type renameCommand struct {
	SpaceCommandBase
	Name    string
	NewName string
}

const renameCommandDoc = `
Renames an existing space from "old-name" to "new-name". Does not change the
associated subnets and "new-name" must not match another existing space.
`

func (c *renameCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SpaceCommandBase.SetFlags(f)
	f.StringVar(&c.NewName, "rename", "", "the new name for the network space")
}

// Info is defined on the cmd.Command interface.
func (c *renameCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "rename",
		Args:    "<old-name> <new-name>",
		Purpose: "rename a network space",
		Doc:     strings.TrimSpace(renameCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *renameCommand) Init(args []string) (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid arguments specified")

	switch len(args) {
	case 0:
		return errors.New("old-name is required")
	case 1:
		return errors.New("new-name is required")
	}
	for _, name := range args {
		if !names.IsValidSpace(name) {
			return errors.Errorf("%q is not a valid space name", name)
		}
	}
	c.Name = args[0]
	c.NewName = args[1]

	if c.Name == c.NewName {
		return errors.New("old-name and new-name are the same")
	}

	return cmd.CheckEmpty(args[2:])
}

// Run implements Command.Run.
func (c *renameCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		err := api.RenameSpace(c.Name, c.NewName)
		if err != nil {
			return errors.Annotatef(err, "cannot rename space %q", c.Name)
		}

		ctx.Infof("renamed space %q to %q", c.Name, c.NewName)
		return nil
	})
}
