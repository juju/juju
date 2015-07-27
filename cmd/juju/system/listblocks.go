// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

// ListBlocksCommand lists all blocks for environments within the system.
type ListBlocksCommand struct {
	envcmd.SysCommandBase
	out    cmd.Output
	api    listBlocksAPI
	apierr error
}

var listBlocksDoc = `List all blocks for environments within the specified system`

// listBlocksAPI defines the methods on the system manager API endpoint
// that the list-blocks command calls.
type listBlocksAPI interface {
	Close() error
	ListBlockedEnvironments() ([]params.EnvironmentBlockInfo, error)
}

// Info implements Command.Info.
func (c *ListBlocksCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-blocks",
		Purpose: "list all blocks within the system",
		Doc:     listBlocksDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ListBlocksCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatTabularBlockedEnvironments,
	})
}

func (c *ListBlocksCommand) getAPI() (listBlocksAPI, error) {
	if c.api != nil {
		return c.api, c.apierr
	}
	return c.NewSystemManagerAPIClient()
}

// Run implements Command.Run
func (c *ListBlocksCommand) Run(ctx *cmd.Context) error {
	api, err := c.getAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to the API")
	}
	defer api.Close()

	envs, err := api.ListBlockedEnvironments()
	if err != nil {
		logger.Errorf("Unable to list blocked environments: %s", err)
		return err
	}
	return c.out.Write(ctx, envs)
}
