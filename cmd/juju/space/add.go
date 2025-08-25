// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// NewAddCommand returns a command used to add a network space.
func NewAddCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&AddCommand{})
}

// AddCommand calls the API to add a new network space.
type AddCommand struct {
	SpaceCommandBase
	Name  string
	CIDRs set.Strings
}

const addCommandDoc = `
Adds a new space with the given name and associates the given
(optional) list of existing subnet CIDRs with it.`

const addCommandExamples = `

Add space ` + "`beta`" + ` with subnet ` + "`172.31.0.0/20`" + `:

    juju add-space beta 172.31.0.0/20
`

// Info is defined on the cmd.Command interface.
func (c *AddCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-space",
		Args:     "<name> [<CIDR1> <CIDR2> ...]",
		Purpose:  "Add a new network space.",
		Doc:      strings.TrimSpace(addCommandDoc),
		Examples: addCommandExamples,
		SeeAlso: []string{
			"spaces",
			"remove-space",
		},
	})
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *AddCommand) Init(args []string) error {
	var err error
	c.Name, c.CIDRs, err = ParseNameAndCIDRs(args, true)
	return err
}

// Run implements Command.Run.
func (c *AddCommand) Run(ctx *cmd.Context) error {
	return c.RunWithSpaceAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		// Prepare a nicer message and proper arguments to use in case
		// there are not CIDRs given.
		var subnetIds []string
		msgSuffix := "no subnets"
		if !c.CIDRs.IsEmpty() {
			subnetIds = c.CIDRs.SortedValues()
			msgSuffix = fmt.Sprintf("subnets %s", strings.Join(subnetIds, ", "))
		}

		// Add the new space.
		// TODO(dimitern): Accept --public|--private and pass it here.
		err := api.AddSpace(c.Name, subnetIds, true)
		if err != nil {
			if errors.IsNotSupported(err) {
				ctx.Infof("cannot add space %q: %v", c.Name, err)
			}
			if params.IsCodeUnauthorized(err) {
				common.PermissionsMessage(ctx.Stderr, "add a space")
			}
			return block.ProcessBlockedError(errors.Annotatef(err, "cannot add space %q", c.Name), block.BlockChange)
		}

		ctx.Infof("added space %q with %s", c.Name, msgSuffix)
		return nil
	})
}
