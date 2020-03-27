// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"fmt"
	"io"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// NewMoveCommand returns a command used to move an existing space to a
// different subnet.
func NewMoveCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&MoveCommand{})
}

// MoveCommand calls the API to attempt to move an existing space to a different
// subnet.
type MoveCommand struct {
	SpaceCommandBase
	out cmd.Output

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
		Args:    "[--format yaml|json] <name> <CIDR1> [ <CIDR2> ...]",
		Purpose: "Update a network space's CIDR.",
		Doc:     strings.TrimSpace(moveCommandDoc),
	})
}

// SetFlags defines the move command flags it wants to offer.
func (c *MoveCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Force, "force", false, "Allow to force a move of subnets to a space even if they are in use on another machine.")
	c.out.AddFlags(f, "human", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.printTabular,
		"human":   c.printHuman,
	})
}

// Init checks the arguments for valid arguments and sets up the command to run.
// Defined on the cmd.Command interface.
func (c *MoveCommand) Init(args []string) error {
	var err error
	c.Name, c.CIDRs, err = ParseNameAndCIDRs(args, false)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run implements Command.Run.
func (c *MoveCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api API, ctx *cmd.Context) error {
		subnetTags, err := c.getSubnetTags(ctx, api, c.CIDRs)
		if err != nil {
			return errors.Trace(err)
		}

		// Name here is checked to be a valid space name in ParseNameAndCIDRs.
		spaceTag := names.NewSpaceTag(c.Name)
		moved, err := api.MoveSubnets(spaceTag, subnetTags, c.Force)
		if err != nil {
			return errors.Annotatef(err, "cannot update space %q", spaceTag.Id())
		}

		changes, err := extractMovementChangeLog(api, subnetTags, moved)
		if err != nil {
			return errors.Annotatef(err, "failed to parse the changes")
		}
		return c.out.Write(ctx, changes)
	})
}

func (c *MoveCommand) getSpaceTag(api SpaceAPI, name string) (names.SpaceTag, error) {
	space, err := api.ShowSpace(name)
	if err != nil {
		return names.SpaceTag{}, errors.Annotatef(err, "failed to get space %q", name)
	}

	return names.NewSpaceTag(space.Space.Id), nil
}

func (c *MoveCommand) getSubnetTags(ctx *cmd.Context, api SubnetAPI, cidrs set.Strings) ([]names.SubnetTag, error) {
	sortedCIDRs := cidrs.SortedValues()

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
				// This should never happen, but considering that we're using
				// CIDRs as IDs, then we should at least tripple check.
				ctx.Warningf("Subnet CIDR %q was not one supplied %v", subnet.CIDR, c.CIDRs.SortedValues())
				continue
			}

			tags = append(tags, names.NewSubnetTag(subnet.ID))
		}
	}

	if len(tags) != cidrs.Size() {
		return nil, errors.Errorf("error getting subnet tags for %s", strings.Join(cidrs.SortedValues(), ","))
	}

	return tags, nil
}

func (c *MoveCommand) printHuman(writer io.Writer, value interface{}) error {
	list, ok := value.([]MovedSpace)
	if !ok {
		return errors.New("unexpected value")
	}

	for _, change := range list {
		_, _ = fmt.Fprintf(writer, "Subnet %s moved from %s to %s", change.CIDR, change.SpaceFrom, change.SpaceTo)
	}

	return nil
}

// printTabular prints the list of spaces in tabular format
func (c *MoveCommand) printTabular(writer io.Writer, value interface{}) error {
	tw := output.TabWriter(writer)

	list, ok := value.([]MovedSpace)
	if !ok {
		return errors.New("unexpected value")
	}

	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true

	table.AddRow("Space From", "Space To", "CIDR")
	for _, s := range list {
		table.AddRow(s.SpaceFrom, s.SpaceTo, s.CIDR)
	}

	table.AddRow("", "", "")
	_, _ = fmt.Fprint(writer, table)

	return errors.Trace(tw.Flush())
}

func extractMovementChangeLog(api SpaceAPI, tags []names.SubnetTag, result params.MoveSubnetsResult) ([]MovedSpace, error) {
	var changes []MovedSpace
	for _, moved := range result.MovedSubnets {
		oldSpaceTag, err := names.ParseSpaceTag(moved.OldSpaceTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		newSpaceTag, err := names.ParseSpaceTag(result.NewSpaceTag)
		if err != nil {
			return nil, errors.Trace(err)
		}

		changes = append(changes, MovedSpace{
			SpaceFrom: oldSpaceTag.Id(),
			SpaceTo:   newSpaceTag.Id(),
			CIDR:      moved.CIDR,
		})
	}

	return changes, nil
}

// MovedSpace represents a CIDR movement from space `a` to space `b`
type MovedSpace struct {
	// SpaceFrom is the name of the space which the CIDR left.
	SpaceFrom string `json:"from" yaml:"from"`

	// SpaceTo is the name of the space which the CIDR goes to.
	SpaceTo string `json:"to" yaml:"to"`

	// CIDR of the subnet that is moving.
	CIDR string `json:"cidr" yaml:"cidr"`
}
