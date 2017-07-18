// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveStorageCommandWithAPI returns a command
// used to remove storage from the model.
func NewRemoveStorageCommandWithAPI() cmd.Command {
	cmd := &removeStorageCommand{}
	cmd.newStorageRemoverCloser = func() (StorageRemoverCloser, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// NewRemoveStorageCommand returns a command
// used to remove storage from the model.
func NewRemoveStorageCommand(new NewStorageRemoverCloserFunc) cmd.Command {
	cmd := &removeStorageCommand{}
	cmd.newStorageRemoverCloser = new
	return modelcmd.Wrap(cmd)
}

const (
	removeStorageCommandDoc = `
Removes storage from the model. Specify one or more
storage IDs, as output by "juju storage".

By default, remove-storage will fail if the storage
is attached to any units. To override this behaviour,
you can use "juju remove-storage --force".

Examples:
    # Remove the detached storage pgdata/0.
    juju remove-storage pgdata/0

    # Remove the possibly attached storage pgdata/0.
    juju remove-storage --force pgdata/0

    # Remove the storage pgdata/0, without destroying
    # the corresponding cloud storage.
    juju remove-storage --no-destroy pgdata/0
`
	removeStorageCommandArgs = `<storage> [<storage> ...]`
)

type removeStorageCommand struct {
	StorageCommandBase
	newStorageRemoverCloser NewStorageRemoverCloserFunc
	storageIds              []string
	force                   bool
	noDestroy               bool
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

func (c *removeStorageCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "Remove storage even if it is currently attached")
	f.BoolVar(&c.noDestroy, "no-destroy", false, "Remove the storage without destroying it")
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
	remover, err := c.newStorageRemoverCloser()
	if err != nil {
		return errors.Trace(err)
	}
	defer remover.Close()

	destroyAttachments := c.force
	destroyStorage := !c.noDestroy
	results, err := remover.Remove(c.storageIds, destroyAttachments, destroyStorage)
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
	anyAttached := false
	for i, result := range results {
		if result.Error != nil {
			ctx.Infof("failed to remove %s: %s", c.storageIds[i], result.Error)
			if params.IsCodeStorageAttached(result.Error) {
				anyAttached = true
			}
			anyFailed = true
		}
	}
	if anyAttached {
		ctx.Infof(`
Use the --force flag to remove attached storage, or use
"juju detach-storage" to explicitly detach the storage
before removing.`)
	}
	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}

// NewStorageRemoverCloserFunc is the type of a function that returns an
// StorageRemoverCloser.
type NewStorageRemoverCloserFunc func() (StorageRemoverCloser, error)

// StorageRemoverCloser extends StorageRemover with a Closer method.
type StorageRemoverCloser interface {
	StorageRemover
	Close() error
}

// StorageRemover defines an interface for destroying storage instances
// with the specified IDs.
type StorageRemover interface {
	Remove(
		storageIds []string,
		destroyAttachments, destroyStorage bool,
	) ([]params.ErrorResult, error)
}
