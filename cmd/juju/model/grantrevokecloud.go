// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/permission"
)

var validCloudAccessLevels = `
Valid access levels are:
    `[1:] + strings.Join(filterAccessLevels(permission.AllAccessLevels, permission.ValidateCloudAccess), "\n    ")

var usageGrantCloudSummary = `
Grants access level to a Juju user for a cloud.`[1:]

var usageGrantCloudDetails = validCloudAccessLevels

const usageGrantCloudExamples = `
Grant user ` + "`joe`" + ` ` + "`add-model`" + ` access to cloud ` + "`fluffy`" + `:

    juju grant-cloud joe add-model fluffy
`

var usageRevokeCloudSummary = `
Revokes access from a Juju user for a cloud.`[1:]

var usageRevokeCloudDetails = `
Revoking admin access, from a user who has that permission, will leave
that user with ` + "`add-model`" + ` access. Revoking ` + "`add-model`" + ` access, however, also revokes
admin access.

`[1:] + validCloudAccessLevels

const usageRevokeCloudExamples = `
Revoke ` + "`add-model`" + ` (and 'admin') access from user ` + "`joe`" + ` for cloud ` + "`fluffy`" + `:

    juju revoke-cloud joe add-model fluffy

Revoke ` + "`admin`" + ` access from user ` + "`sam`" + ` for clouds ` + "`fluffy`" + ` and ` + "`rainy`" + `:

    juju revoke-cloud sam admin fluffy rainy

`

type accessCloudCommand struct {
	modelcmd.ControllerCommandBase

	User   string
	Clouds []string
	Access string
}

// Init implements cmd.Command.
func (c *accessCloudCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no user specified")
	}

	if len(args) < 2 {
		return errors.New("no permission level specified")
	}

	c.User = args[0]
	c.Access = args[1]
	// The remaining args are cloud names.
	for _, arg := range args[2:] {
		if !names.IsValidCloud(arg) {
			return errors.NotValidf("cloud name %q", arg)
		}
		c.Clouds = append(c.Clouds, arg)
	}

	// Special case for backwards compatibility.
	if c.Access == "addmodel" {
		c.Access = "add-model"
	}
	if len(c.Clouds) > 0 {
		return permission.ValidateCloudAccess(permission.Access(c.Access))
	}
	return errors.Errorf("You need to specify one or more cloud names.\n" +
		"See 'juju help grant-cloud'.")
}

// NewGrantCloudCommand returns a new grant command.
func NewGrantCloudCommand() cmd.Command {
	return modelcmd.WrapController(&grantCloudCommand{})
}

// grantCloudCommand represents the command to grant a user access to one or more clouds.
type grantCloudCommand struct {
	accessCloudCommand
	cloudsApi GrantCloudAPI
}

// Info implements Command.Info.
func (c *grantCloudCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "grant-cloud",
		Args:     "<user name> <permission> <cloud name> ...",
		Purpose:  usageGrantCloudSummary,
		Doc:      usageGrantCloudDetails,
		Examples: usageGrantCloudExamples,
		SeeAlso: []string{
			"grant",
			"revoke-cloud",
			"add-user",
		},
	})
}

func (c *grantCloudCommand) getCloudsAPI() (GrantCloudAPI, error) {
	if c.cloudsApi != nil {
		return c.cloudsApi, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloud.NewClient(root), nil
}

// GrantCloudAPI defines the API functions used by the grant command.
type GrantCloudAPI interface {
	Close() error
	GrantCloud(user, access string, clouds ...string) error
}

// Run implements cmd.Command.
func (c *grantCloudCommand) Run(ctx *cmd.Context) error {
	client, err := c.getCloudsAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.GrantCloud(c.User, c.Access, c.Clouds...), block.BlockChange)
}

// NewRevokeCloudCommand returns a new revoke command.
func NewRevokeCloudCommand() cmd.Command {
	return modelcmd.WrapController(&revokeCloudCommand{})
}

// revokeCloudCommand revokes a user's access to clouds.
type revokeCloudCommand struct {
	accessCloudCommand
	cloudsApi RevokeCloudAPI
}

// Info implements cmd.Command.
func (c *revokeCloudCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "revoke-cloud",
		Args:     "<user name> <permission> <cloud name> ...",
		Purpose:  usageRevokeCloudSummary,
		Doc:      usageRevokeCloudDetails,
		Examples: usageRevokeCloudExamples,
		SeeAlso: []string{
			"grant-cloud",
		},
	})
}

func (c *revokeCloudCommand) getCloudAPI() (RevokeCloudAPI, error) {
	if c.cloudsApi != nil {
		return c.cloudsApi, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloud.NewClient(root), nil
}

// RevokeCloudAPI defines the API functions used by the revoke cloud command.
type RevokeCloudAPI interface {
	Close() error
	RevokeCloud(user, access string, clouds ...string) error
}

// Run implements cmd.Command.
func (c *revokeCloudCommand) Run(ctx *cmd.Context) error {
	client, err := c.getCloudAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.RevokeCloud(c.User, c.Access, c.Clouds...), block.BlockChange)
}
