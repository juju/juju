// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

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
}

// TrackBranchCommandAPI describes API methods required
// to execute the track command.
//go:generate mockgen -package mocks -destination ./mocks/trackbranch_mock.go github.com/juju/juju/cmd/juju/model TrackBranchCommandAPI
type TrackBranchCommandAPI interface {
	Close() error

	// TrackBranch sets the input units and/or applications
	// to track changes made under the input branch name.
	TrackBranch(branchName string, entities []string) error
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
}

// Init implements part of the cmd.Command interface.
func (c *trackBranchCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("expected a branch name plus unit and/or application names(s)")
	}

	for _, arg := range args[1:] {
		if !names.IsValidApplication(arg) && !names.IsValidUnit(arg) {
			return errors.Errorf("invalid application or unit name %q", arg)
		}
	}
	c.branchName = args[0]
	c.entities = args[1:]
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
		} else {
			return errors.Errorf("expected unit and/or application names(s)")
		}
	}

	return errors.Trace(client.TrackBranch(c.branchName, c.entities))
}
