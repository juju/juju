// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var helpSummaryResolved = `
Marks all units in error as resolved.`[1:]

var helpDetailsResolved = `
Examples:
    juju resolve wordpress
    juju resolve mysql wordpress --retry
    juju resolve --all
`

// NewResolvedCommand returns a command to resolve units in error status.
func NewResolvedCommand() cmd.Command {
	cmd := &resolvedCommand{}
	cmd.newAPIFunc = func() (ResolvedAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return application.NewClient(root), nil
	}
	return modelcmd.Wrap(cmd)
}

// NewResolvedCommandForTest returns an NewResolvedCommand with the api provided as specified.
func NewResolvedCommandForTest(api ResolvedAPI) modelcmd.ModelCommand {
	cmd := &resolvedCommand{newAPIFunc: func() (ResolvedAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}

// resolvedCommand marks a unit in an error state as ready to continue.
type resolvedCommand struct {
	modelcmd.ModelCommandBase
	newAPIFunc func() (ResolvedAPI, error)

	UnitTags []string
	NoRetry  bool
	all      bool
}

// Info is part of cmd.Resolved interface
func (c *resolvedCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "resolved",
		Args:    "<unit1> [<unit2>] [--retry] | --all [--retry]",
		Purpose: helpSummaryResolved,
		Doc:     helpDetailsResolved,
	}
}

func (c *resolvedCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.all, "all", false, "Marks all units in error as resolved")
	f.BoolVar(&c.NoRetry, "no-retry", false, "Do not re-execute failed hooks on the unit")
}

func (c *resolvedCommand) Init(args []string) (err error) {
	if len(args) > 0 && c.all == true {
		return errors.Errorf("specify unit or --all option, not both")
	}
	if len(args) > 0 {
		for _, unit := range c.UnitTags {
			if !names.IsValidUnit(unit) {
				return errors.Errorf("invalid unit name %q", c.UnitTags)
			}
			args = args[1:]
		}
	} else if c.all == false {
		return errors.Errorf("no unit specified")
	}
	return cmd.CheckEmpty(args)
}

// ResolvedAPI defines the API methods that the application resolved command.
type ResolvedAPI interface {
	Close() error
	BestAPIVersion() int

	// This method is supported in API < V6
	Resolved(string, bool) error

	// These methods are on API V6.
	ResolveApplicationUnits([]string, bool, bool) error
}

// Run implements the cmd.Resolved interface
func (c *resolvedCommand) Run(_ *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()
	if client.BestAPIVersion() < 6 {
		if len(c.UnitTags) > 0 || c.all == true {
			return errors.New("resolve with multiple units is not supported by this version of Juju")
		}
		err = client.Resolved(c.UnitTags[0], c.NoRetry)
	} else {
		err = client.ResolveApplicationUnits(c.UnitTags, c.NoRetry, c.all)
	}
	return block.ProcessBlockedError(err, block.BlockChange)
}
