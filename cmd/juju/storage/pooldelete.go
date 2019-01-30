// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

// PoolDeleteAPI defines the API methods that the storage commands use.
type PoolDeleteAPI interface {
	Close() error
	DeletePool(name string) error
	BestAPIVersion() int
}

const poolDeleteCommandDoc = `
Delete a single existing storage pool.

Example:
    # Delete the storage-pool named operator-storage

      juju delete-storage-pool operator-storage

See also:
    create-storage-pool
    update-storage-pool
    storage-pools
`

// NewPoolDeleteCommand returns a command that deletes the named storage pool.
func NewPoolDeleteCommand() cmd.Command {
	cmd := &poolDeleteCommand{}
	cmd.newAPIFunc = func() (PoolDeleteAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// poolDeleteCommand deletes a storage pool.
type poolDeleteCommand struct {
	PoolCommandBase
	newAPIFunc func() (PoolDeleteAPI, error)
	poolName   string
}

// Init implements Command.Init.
func (c *poolDeleteCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("pool deletion requires storage pool name")
	}
	c.poolName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Info implements Command.Info.
func (c *poolDeleteCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "delete-storage-pool",
		Purpose: "Delete an existing storage pool.",
		Doc:     poolDeleteCommandDoc,
		Args:    "<name>",
	})
}

// Run implements Command.Run.
func (c *poolDeleteCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()
	if api.BestAPIVersion() < 5 {
		return errors.New("deleting storage pools is not supported by this version of Juju")
	}
	return api.DeletePool(c.poolName)
}
