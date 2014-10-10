package action

import (
	"errors"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

type undefinedActionCommand struct {
	ActionCommandBase
}

func (c *undefinedActionCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "undefined",
		Args:    "undefined",
		Purpose: "undefined",
		Doc:     "undefined",
	}
}

func (c *undefinedActionCommand) Init(args []string) error {
	return errors.New("This command is not yet implemented!")
}

func (c *undefinedActionCommand) SetFlags(f *gnuflag.FlagSet) {
}

func (c *undefinedActionCommand) Run(ctx *cmd.Context) error {
	return errors.New("This command is not yet implemented!")
}
