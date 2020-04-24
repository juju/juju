// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	trackBranchSummary = "Set units and/or applications to realise changes made under a branch."
	trackBranchDoc     = `
Specific units can be set to track a branch by supplying multiple unit IDs.
All units of an application can be set to track a branch by passing an
application name. Units can only track one branch at a time.

Examples:
    juju track test-branch redis/0
    juju track test-branch redis
    juju track test-branch redis -n 2
    juju track test-branch redis/0 mysql

See also:
    add-branch
    branch
    commit
    abort
    diff
`
)

// NewTrackBranchCommand wraps trackBranchCommand with sane model settings.
func NewTrackBranchCommand() cmd.Command {
	return modelcmd.Wrap(&trackBranchCommand{})
}

// trackBranchCommand supplies the "track" CLI command used to make units
// realise changes made under a branch.
type trackBranchCommand struct {
	modelcmd.ModelCommandBase

	api TrackBranchCommandAPI

	branchName string
	entities   []string

	// numUnits describes the number of units to track. A strategy will be
	// picked to track the number of units if there are more than the number
	// requested.
	numUnits autoIntValue
}

// TrackBranchCommandAPI describes API methods required
// to execute the track command.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination ./mocks/trackbranch_mock.go github.com/juju/juju/cmd/juju/model TrackBranchCommandAPI
type TrackBranchCommandAPI interface {
	Close() error

	// TrackBranch sets the input units and/or applications
	// to track changes made under the input branch name.
	TrackBranch(branchName string, entities []string, numUnits int) error
	HasActiveBranch(branchName string) (bool, error)
}

// Info implements part of the cmd.Command interface.
func (c *trackBranchCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "track",
		Args:    "<branch name> <entities> ...",
		Purpose: trackBranchSummary,
		Doc:     trackBranchDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *trackBranchCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.Var(&c.numUnits, "n", "The number of units to track")
}

// Init implements part of the cmd.Command interface.
func (c *trackBranchCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("expected a branch name plus unit and/or application names(s)")
	}
	// If the operator suggested that they wanted to track -n 0, or do it via
	// automated code, then that will attempt to track all units, which is
	// probably _not_ what they envisaged.
	// So here we trap the potential error and state that if the operator passes
	// 0, we report an error and if they pass less than 0, we can assume they
	// mean all units.
	var flagNumUnits int
	if c.numUnits.v != nil {
		flagNumUnits = *c.numUnits.v
		if flagNumUnits <= 0 {
			return errors.Errorf("expected a valid number of units to track")
		}
	}
	c.numUnits.v = &flagNumUnits

	var numUnits int
	var numApplications int

	entities := args[1:]
	for _, arg := range entities {
		validApplication := names.IsValidApplication(arg)
		validUnit := names.IsValidUnit(arg)
		if !validApplication && !validUnit {
			return errors.Errorf("invalid application or unit name %q", arg)
		}

		if validApplication {
			numApplications++
		}
		if validUnit {
			numUnits++
		}
	}
	// If the number of units the user requested is greater than 0, then we
	// need to block asking for multiple applications. This is because we don't
	// know how to topographically distribute between all the applications and
	// units, especially if an error occurs whilst assigning the units.
	// To prevent that issue happening, guard against it.
	if *c.numUnits.v > 0 {
		if numApplications+numUnits > 1 {
			return errors.Errorf("-n flag not allowed when specifying multiple units and/or applications")
		}
		// If the number of entites is 1, but you've requested a unit, then this
		// is implicit, but not really required.
		if numUnits > 0 {
			return errors.Errorf("-n flag not allowed when specifying units")
		}
	}
	c.branchName = args[0]
	c.entities = entities
	return nil
}

// getAPI returns the API that supplies methods
// required to execute this command.
func (c *trackBranchCommand) getAPI() (TrackBranchCommandAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	client := modelgeneration.NewClient(api)
	return client, nil
}

// Run implements the meaty part of the cmd.Command interface.
func (c *trackBranchCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	defer func() { _ = client.Close() }()

	if err != nil {
		return err
	}

	if len(c.entities) == 0 {
		isActiveBranch, err := client.HasActiveBranch(c.branchName)
		if err != nil {
			return errors.Annotate(err, "checking for active branch")
		}
		if !isActiveBranch {
			return errors.NotFoundf("branch %q", c.branchName)
		}
		return errors.Errorf("expected unit and/or application names(s)")
	}

	return errors.Trace(client.TrackBranch(c.branchName, c.entities, *c.numUnits.v))
}

// autoIntValue allows the value of nil to mean something when attempting
// to inspect a flag. The following just ensures that the default value is nil
// and any new value once set valid for validation.
type autoIntValue struct {
	v *int
}

func (i *autoIntValue) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return err
	}
	num := int(v)
	i.v = &num
	return nil
}

func (i *autoIntValue) Get() interface{} {
	if i.v != nil {
		return *i.v
	}
	return i.v // nil
}

func (i *autoIntValue) String() string {
	if i.v == nil {
		return "<auto>"
	}
	return fmt.Sprintf("%v", *i.v)
}
