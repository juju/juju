// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/cmd/envcmd"
)

func newCreateCommand() cmd.Command {
	return envcmd.Wrap(&createCommand{})
}

// createCommand calls the API to create a new network space.
type createCommand struct {
	SpaceCommandBase
	Name  string
	CIDRs set.Strings
}

const createCommandDoc = `
Creates a new space with the given name and associates the given
(optional) list of existing subnet CIDRs with it.`

// Info is defined on the cmd.Command interface.
func (c *createCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<name> [<CIDR1> <CIDR2> ...]",
		Purpose: "create a new network space",
		Doc:     strings.TrimSpace(createCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *createCommand) Init(args []string) error {
	var err error
	c.Name, c.CIDRs, err = ParseNameAndCIDRs(args, true)
	return err
}

// Run implements Command.Run.
func (c *createCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		// Prepare a nicer message and proper arguments to use in case
		// there are not CIDRs given.
		var subnetIds []string
		msgSuffix := "no subnets"
		if !c.CIDRs.IsEmpty() {
			subnetIds = c.CIDRs.SortedValues()
			msgSuffix = fmt.Sprintf("subnets %s", strings.Join(subnetIds, ", "))
		}

		// Create the new space.
		// TODO(dimitern): Accept --public|--private and pass it here.
		err := api.CreateSpace(c.Name, subnetIds, true)
		if err != nil {
			if errors.IsNotSupported(err) {
				ctx.Infof("cannot create space %q: %v", c.Name, err)
			}
			return errors.Annotatef(err, "cannot create space %q", c.Name)
		}

		ctx.Infof("created space %q with %s", c.Name, msgSuffix)
		return nil
	})
}
