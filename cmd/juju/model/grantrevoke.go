// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/permission"
)

var usageGrantSummary = `
Grants access to a Juju user for a model.`[1:]

var usageGrantDetails = `
By default, the controller is the current controller.
Model access can also be granted at user-addition time with the `[1:] + "`juju add-\nuser`" + ` command.
Users with read access are limited in what they can do with models: ` + "`juju \nlist-models`, `juju list-machines`, and `juju status`" + `.

Examples:
Grant user 'joe' default (read) access to model 'mymodel':

    juju grant joe mymodel

Grant user 'jim' write access to model 'mymodel':

    juju grant --acl=write jim mymodel

Grant user 'sam' default (read) access to models 'model1' and 'model2':

    juju grant sam model1 model2

See also: 
    revoke
    add-user`

var usageRevokeSummary = `
Revokes access from a Juju user for a model.`[1:]

var usageRevokeDetails = `
By default, the controller is the current controller.
Revoking write access, from a user who has that permission, will leave
that user with read access. Revoking read access, however, also revokes
write access.

Examples:
Revoke read (and write) access from user 'joe' for model 'mymodel':

    juju revoke joe mymodel

Revoke write access from user 'sam' for models 'model1' and 'model2':

    juju revoke --acl=write sam model1 model2

See also: 
    grant`[1:]

type accessCommand struct {
	modelcmd.ControllerCommandBase

	User        string
	ModelNames  []string
	ModelAccess string
}

// SetFlags implements cmd.Command.
func (c *accessCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.ModelAccess, "acl", "read", "Access control ('read' or 'write')")
}

// Init implements cmd.Command.
func (c *accessCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no user specified")
	}

	if len(args) < 2 {
		return errors.New("no model specified")
	}

	_, err := permission.ParseModelAccess(c.ModelAccess)
	if err != nil {
		return err
	}

	c.User = args[0]
	c.ModelNames = args[1:]
	return nil
}

// NewGrantCommand returns a new grant command.
func NewGrantCommand() cmd.Command {
	return modelcmd.WrapController(&grantCommand{})
}

// grantCommand represents the command to grant a user access to one or more models.
type grantCommand struct {
	accessCommand
	api GrantModelAPI
}

// Info implements Command.Info.
func (c *grantCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "grant",
		Args:    "<user name> <model name> ...",
		Purpose: usageGrantSummary,
		Doc:     usageGrantDetails,
	}
}

func (c *grantCommand) getAPI() (GrantModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewModelManagerAPIClient()
}

// GrantModelAPI defines the API functions used by the grant command.
type GrantModelAPI interface {
	Close() error
	GrantModel(user, access string, modelUUIDs ...string) error
}

// Run implements cmd.Command.
func (c *grantCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	models, err := c.ModelUUIDs(c.ModelNames)
	if err != nil {
		return err
	}
	return block.ProcessBlockedError(client.GrantModel(c.User, c.ModelAccess, models...), block.BlockChange)
}

// NewRevokeCommand returns a new revoke command.
func NewRevokeCommand() cmd.Command {
	return modelcmd.WrapController(&revokeCommand{})
}

// revokeCommand revokes a user's access to models.
type revokeCommand struct {
	accessCommand
	api RevokeModelAPI
}

// Info implements cmd.Command.
func (c *revokeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "revoke",
		Args:    "<user> <model name> ...",
		Purpose: usageRevokeSummary,
		Doc:     usageRevokeDetails,
	}
}

func (c *revokeCommand) getAPI() (RevokeModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewModelManagerAPIClient()
}

// RevokeModelAPI defines the API functions used by the revoke command.
type RevokeModelAPI interface {
	Close() error
	RevokeModel(user, access string, modelUUIDs ...string) error
}

// Run implements cmd.Command.
func (c *revokeCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	modelUUIDs, err := c.ModelUUIDs(c.ModelNames)
	if err != nil {
		return err
	}
	return block.ProcessBlockedError(client.RevokeModel(c.User, c.ModelAccess, modelUUIDs...), block.BlockChange)
}
