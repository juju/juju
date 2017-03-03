// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveStorageCommandWithAPI returns a command
// used to remove storage from the model.
func NewRemoveStorageCommandWithAPI() cmd.Command {
	cmd := &removeStorageCommand{}
	cmd.newEntityDestroyerCloser = func() (EntityDestroyerCloser, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// NewRemoveStorageCommand returns a command
// used to remove storage from the model.
func NewRemoveStorageCommand(new NewEntityDestroyerCloserFunc) cmd.Command {
	cmd := &removeStorageCommand{}
	cmd.newEntityDestroyerCloser = new
	return modelcmd.Wrap(cmd)
}

const (
	removeStorageCommandDoc = `
Removes storage from the model. Specify one or more
storage IDs, as output by "juju storage".

Examples:
    juju remove-storage pgdata/0
`
	removeStorageCommandArgs = `<storage> [<storage> ...]`
)

type removeStorageCommand struct {
	StorageCommandBase
	newEntityDestroyerCloser NewEntityDestroyerCloserFunc
	storageIds               []string
}

// Info implements Command.Info.
func (c *removeStorageCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-storage",
		Purpose: "Removes storage from the model.",
		Doc:     removeStorageCommandDoc,
		Args:    removeStorageCommandArgs,
	}
}

// Init implements Command.Init.
func (c *removeStorageCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("remove-storage requires at least one storage ID")
	}
	c.storageIds = args
	return nil
}

// Run implements Command.Run.
func (c *removeStorageCommand) Run(ctx *cmd.Context) error {
	destroyer, err := c.newEntityDestroyerCloser()
	if err != nil {
		return errors.Trace(err)
	}
	defer destroyer.Close()

	results, err := destroyer.Destroy(c.storageIds)
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			common.PermissionsMessage(ctx.Stderr, "remove storage")
		}
		return err
	}
	for i, result := range results {
		if result.Error == nil {
			ctx.Infof("removing %s", c.storageIds[i])
		}
	}
	anyFailed := false
	for i, result := range results {
		if result.Error != nil {
			ctx.Infof("failed to remove %s: %s", c.storageIds[i], result.Error)
			anyFailed = true
		}
	}
	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}

// NewEntityDestroyerCloserFunc is the type of a function that returns an
// EntityDestroyerCloser.
type NewEntityDestroyerCloserFunc func() (EntityDestroyerCloser, error)

// EntityDestroyerCloser extends EntityDestroyer with a Closer method.
type EntityDestroyerCloser interface {
	EntityDestroyer
	Close() error
}

// EntityDestroyer defines an interface for destroying storage instances
// with the specified IDs.
type EntityDestroyer interface {
	Destroy([]string) ([]params.ErrorResult, error)
}
