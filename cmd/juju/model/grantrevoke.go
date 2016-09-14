// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/permission"
)

var usageGrantSummary = `
Grants access level to a Juju user for a model or controller.`[1:]

var usageGrantDetails = `
By default, the controller is the current controller.

Users with read access are limited in what they can do with models:
` + "`juju models`, `juju machines`, and `juju status`" + `.

Examples:
Grant user 'joe' 'read' access to model 'mymodel':

    juju grant joe read mymodel

Grant user 'jim' 'write' access to model 'mymodel':

    juju grant jim write mymodel

Grant user 'sam' 'read' access to models 'model1' and 'model2':

    juju grant sam read model1 model2

Grant user 'maria' 'addmodel' access to the controller:

    juju grant maria addmodel

Valid access levels for models are:
    read
    write
    admin

Valid access levels for controllers are:
    login
    addmodel
    superuser

See also: 
    revoke
    add-user`

var usageRevokeSummary = `
Revokes access from a Juju user for a model or controller`[1:]

var usageRevokeDetails = `
By default, the controller is the current controller.

Revoking write access, from a user who has that permission, will leave
that user with read access. Revoking read access, however, also revokes
write access.

Examples:
Revoke 'read' (and 'write') access from user 'joe' for model 'mymodel':

    juju revoke joe read mymodel

Revoke 'write' access from user 'sam' for models 'model1' and 'model2':

    juju revoke sam write model1 model2

Revoke 'addmodel' acces from user 'maria' to the controller:

    juju revoke maria addmodel

See also: 
    grant`[1:]

type accessCommand struct {
	modelcmd.ControllerCommandBase

	User        string
	ModelNames  []string
	ModelAccess string
}

// Init implements cmd.Command.
func (c *accessCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no user specified")
	}

	if len(args) < 2 {
		return errors.New("no permission level specified")
	}

	c.User = args[0]
	c.ModelNames = args[2:]
	c.ModelAccess = args[1]
	if len(c.ModelNames) > 0 {
		err := permission.ValidateModelAccess(permission.Access(c.ModelAccess))
		if err != nil {
			return err
		}
	}
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
		Args:    "<user name> <permission> [<model name> ...]",
		Purpose: usageGrantSummary,
		Doc:     usageGrantDetails,
	}
}

func (c *grantCommand) getModelAPI() (GrantModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *grantCommand) getControllerAPI() (GrantControllerAPI, error) {
	return c.NewControllerAPIClient()
}

// GrantModelAPI defines the API functions used by the grant command.
type GrantModelAPI interface {
	Close() error
	GrantModel(user, access string, modelUUIDs ...string) error
}

// GrantControllerAPI defines the API functions used by the grant command.
type GrantControllerAPI interface {
	Close() error
	GrantController(user, access string) error
}

// Run implements cmd.Command.
func (c *grantCommand) Run(ctx *cmd.Context) error {
	if len(c.ModelNames) > 0 {
		return c.runForModel()
	}
	return c.runForController()
}

func (c *grantCommand) runForController() error {
	client, err := c.getControllerAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.GrantController(c.User, c.ModelAccess), block.BlockChange)
}

func (c *grantCommand) runForModel() error {
	client, err := c.getModelAPI()
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
		Args:    "<user> <permission> [<model name> ...]",
		Purpose: usageRevokeSummary,
		Doc:     usageRevokeDetails,
	}
}

func (c *revokeCommand) getModelAPI() (RevokeModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *revokeCommand) getControllerAPI() (RevokeControllerAPI, error) {
	return c.NewControllerAPIClient()
}

// RevokeModelAPI defines the API functions used by the revoke command.
type RevokeModelAPI interface {
	Close() error
	RevokeModel(user, access string, modelUUIDs ...string) error
}

// RevokeControllerAPI defines the API functions used by the revoke command.
type RevokeControllerAPI interface {
	Close() error
	RevokeController(user, access string) error
}

// Run implements cmd.Command.
func (c *revokeCommand) Run(ctx *cmd.Context) error {
	if len(c.ModelNames) > 0 {
		return c.runForModel()
	}
	return c.runForController()
}

func (c *revokeCommand) runForController() error {
	client, err := c.getControllerAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.RevokeController(c.User, c.ModelAccess), block.BlockChange)
}

func (c *revokeCommand) runForModel() error {
	client, err := c.getModelAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	models, err := c.ModelUUIDs(c.ModelNames)
	if err != nil {
		return err
	}
	return block.ProcessBlockedError(client.RevokeModel(c.User, c.ModelAccess, models...), block.BlockChange)
}
