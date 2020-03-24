// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"gopkg.in/juju/names.v3"
)

// NewMoveCommand returns a command used to move an existing space to a
// different subnet.

// MoveCommand calls the API to attempt to move an existing space to a different
// subnet.
type MoveCommand struct {
	SpaceCommandBase

	Name  string
	CIDRs set.Strings

	Force bool
}

const moveCommandDoc = `
Replaces the list of associated subnets of the space. Since subnets
can only be part of a single space, all specified subnets (using their
CIDRs) "leave" their current space and "enter" the one we're updating.

Examples:

Move a list of CIDRs from their space to a new space:
	juju move-to-space db-space 172.31.1.0/28 172.31.16.0/20

See also:
	add-space
	list-spaces
	reload-spaces
	rename-space
	show-space
	remove-space
`

// Info returns a cmd.Info that details the move command information.
func (c *MoveCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "move-to-space",
		Args:    "<name> <CIDR1> [ <CIDR2> ...]",
		Purpose: "Update a network space's CIDR.",
		Doc:     strings.TrimSpace(moveCommandDoc),
	})
}

// SetFlags defines the move command flags it wants to offer.
func (c *MoveCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Force, "force", false, "Allow to force a move of subnets to a space even if they are in use on another machine.")
}

// Init checks the arguments for valid arguments and sets up the command to run.
// Defined on the cmd.Command interface.
func (c *MoveCommand) Init(args []string) error {
	var err error
	c.Name, c.CIDRs, err = ParseNameAndCIDRs(args, false)
	return errors.Trace(err)
}

// Run implements Command.Run.
func (c *MoveCommand) Run(ctx *cmd.Context) error {
	name, err := names.ParseSpaceTag(c.Name)
	if err != nil {
		return errors.Annotatef(err, "expected valid space tag")
	}

	return c.RunWithAPI(ctx, func(api API, ctx *cmd.Context) error {
		subnetTags, err := c.gatherSubnetTags(api)
		if err != nil {
			return errors.Trace(err)
		}

		moved, err := api.MoveSubnets(name, subnetTags, c.Force)
		if err != nil {
			return errors.Annotatef(err, "cannot update space %q", c.Name)
		}

		changes, err := extractMovementChangeLog(subnetTags, moved)
		if err != nil {
			return errors.Annotatef(err, "failed to parse the changes")
		}
		for _, change := range changes {
			ctx.Infof("Subnet %s moved from %s to %s", change.CIDR, change.SpaceFrom, change.SpaceTo)
		}

		return nil
	})
}

func (c *MoveCommand) gatherSubnetTags(api API) ([]names.SubnetTag, error) {
	sortedCIDRs := c.CIDRs.SortedValues()

	subnetResults, err := api.SubnetsByCIDR(sortedCIDRs)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to get subnets by CIDR")
	}

	var tags []names.SubnetTag
	for _, result := range subnetResults {
		if result.Error != nil {
			return nil, errors.Annotatef(result.Error, "error getting subnet by CIDR")
		}

		for _, subnet := range result.Subnets {
			// If the resulting subnet CIDR isn't what we expect, we just skip.
			// The changelog results should highlight that this was a problem,
			// i.e. it wasn't changed.
			if !c.CIDRs.Contains(subnet.CIDR) {
				continue
			}

			tags = append(tags, names.NewSubnetTag(subnet.ID))
		}
	}
	return tags, nil
}

func extractMovementChangeLog(tags []names.SubnetTag, result params.MoveSubnetsResult) ([]MovedSpace, error) {
	var changes []MovedSpace
	for _, moved := range result.MovedSubnets {
		tagFrom, err := names.ParseSpaceTag(moved.OldSpaceTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		tagTo, err := names.ParseSpaceTag(moved.SubnetTag)
		if err != nil {
			return nil, errors.Trace(err)
		}

		changes = append(changes, MovedSpace{
			SpaceFrom: tagFrom.Id(),
			SpaceTo:   tagTo.Id(),
			CIDR:      moved.CIDR,
		})
	}

	return changes, nil
}

// MovedSpace represents a CIDR movement from space `a` to space `b`
type MovedSpace struct {
	// SpaceFrom is the name of the space which the CIDR left.
	SpaceFrom string

	// CIDR of the subnet that is moving.
	CIDR string

	// SpaceTo is the name of the space which the CIDR goes to.
	SpaceTo string
}
