// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// NewRemoveStorageCommandWithAPI returns a command
// used to remove storage from the model.
func NewRemoveStorageCommandWithAPI() cmd.Command {
	command := &removeStorageCommand{}
	command.newStorageRemoverCloser = func(ctx context.Context) (StorageRemoverCloser, error) {
		return command.NewStorageAPI(ctx)
	}
	return modelcmd.Wrap(command)
}

const (
	removeStorageCommandDoc = `
Remove a storage instance from the model and request destruction of its
backing. Use ` + "`--no-destroy`" + ` to release the backing. Both forms remove the
Juju instance ID. Record the provider ID first if you need to find the backing later.

Wait for detachment to finish. On machine models, ` + "`--force`" + ` requests
detachment before removal. Check the provider inventory to confirm what remains.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/
`
	removeStorageCommandExamples = `
Release the detached pgdata/0 filesystem after recording its provider ID:

    juju remove-storage --no-destroy pgdata/0

Remove the detached pgdata/0 instance and request destruction of its backing
after verifying that you no longer need its data:

    juju remove-storage pgdata/0
`

	removeStorageCommandArgs = `<storage> [<storage> ...]`
)

type removeStorageCommand struct {
	StorageCommandBase
	newStorageRemoverCloser NewStorageRemoverCloserFunc
	storageIds              []string
	force                   bool
	noDestroy               bool
	NoWait                  bool
	modelType               model.ModelType
	fs                      *gnuflag.FlagSet
}

// Info implements Command.Info.
func (c *removeStorageCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-storage",
		Purpose:  "Removes storage from the model.",
		Doc:      removeStorageCommandDoc,
		Args:     removeStorageCommandArgs,
		Examples: removeStorageCommandExamples,
		SeeAlso: []string{
			"add-storage",
			"attach-storage",
			"detach-storage",
			"show-storage",
			"storage",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeStorageCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "Request detachment before removal (machine models only)")
	f.BoolVar(&c.noDestroy, "no-destroy", false, "Remove the storage without destroying it")
	c.fs = f
}

// Init implements Command.Init.
func (c *removeStorageCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("remove-storage requires at least one storage ID")
	}
	var err error
	if c.modelType, err = c.ModelType(context.TODO()); err != nil {
		return errors.Trace(err)
	}
	if c.modelType == model.CAAS && c.force {
		return errors.NotSupportedf("forced detachment of storage on container models")
	}

	c.storageIds = args
	return nil
}

// Run implements Command.Run.
func (c *removeStorageCommand) Run(ctx *cmd.Context) error {
	noWaitSet := false
	forceSet := false
	c.fs.Visit(func(flag *gnuflag.Flag) {
		if flag.Name == "no-wait" {
			noWaitSet = true
		} else if flag.Name == "force" {
			forceSet = true
		}
	})
	if !forceSet && noWaitSet {
		return errors.NotValidf("--no-wait without --force")
	}
	var maxWait *time.Duration
	if c.force {
		if c.NoWait {
			zeroSec := 0 * time.Second
			maxWait = &zeroSec
		}
	}

	remover, err := c.newStorageRemoverCloser(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer remover.Close()

	destroyAttachments := c.force
	destroyStorage := !c.noDestroy
	results, err := remover.Remove(ctx, c.storageIds, destroyAttachments, destroyStorage, &c.force, maxWait)
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
	if anyAttached && c.modelType != model.CAAS {
		ctx.Infof(`
Use the --force option to remove attached storage, or use
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
type NewStorageRemoverCloserFunc func(ctx context.Context) (StorageRemoverCloser, error)

// StorageRemoverCloser extends StorageRemover with a Closer method.
type StorageRemoverCloser interface {
	StorageRemover
	Close() error
}

// StorageRemover defines an interface for destroying storage instances
// with the specified IDs.
type StorageRemover interface {
	Remove(
		ctx context.Context,
		storageIds []string,
		destroyAttachments, destroyStorage bool,
		force *bool, maxWait *time.Duration,
	) ([]params.ErrorResult, error)
}
