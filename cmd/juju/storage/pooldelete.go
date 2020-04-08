// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

// PoolRemoveAPI defines the API methods that the storage commands use.
type PoolRemoveAPI interface {
	Close() error
	RemovePool(name string) error
	BestAPIVersion() int
}

const poolRemoveCommandDoc = `
Remove a single existing storage pool.

Example:
    # Remove the storage-pool named fast-storage

      juju remove-storage-pool fast-storage

See also:
    create-storage-pool
    update-storage-pool
    storage-pools
`

// NewPoolRemoveCommand returns a command that removes the named storage pool.
func NewPoolRemoveCommand() cmd.Command {
	cmd := &poolRemoveCommand{}
	cmd.newAPIFunc = func() (PoolRemoveAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// poolRemoveCommand removes a storage pool.
type poolRemoveCommand struct {
	PoolCommandBase
	newAPIFunc func() (PoolRemoveAPI, error)
	poolName   string
}

// Init implements Command.Init.
func (c *poolRemoveCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("pool removal requires storage pool name")
	}
	c.poolName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Info implements Command.Info.
func (c *poolRemoveCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-storage-pool",
		Purpose: "Remove an existing storage pool.",
		Doc:     poolRemoveCommandDoc,
		Args:    "<name>",
	})
}

// Run implements Command.Run.
func (c *poolRemoveCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()
	if api.BestAPIVersion() < 5 {
		return errors.New("removing storage pools is not supported by this version of Juju")
	}
	err = api.RemovePool(c.poolName)
	if params.IsCodeNotFound(err) {
		ctx.Infof("removing storage pool %s failed: %s", c.poolName, err)
		return cmd.ErrSilent
	}
	return err
}
