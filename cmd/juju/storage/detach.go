// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/rpc/params"
)

// NewDetachStorageCommandWithAPI returns a command
// used to detach storage from application units.
func NewDetachStorageCommandWithAPI() cmd.Command {
	command := &detachStorageCommand{}
	command.newEntityDetacherCloser = func(ctx context.Context) (EntityDetacherCloser, error) {
		return command.NewStorageAPI(ctx)
	}
	return modelcmd.Wrap(command)
}

// NewDetachStorageCommand returns a command used to
// detach storage from application units.
func NewDetachStorageCommand(new NewEntityDetacherCloserFunc) cmd.Command {
	command := &detachStorageCommand{}
	command.newEntityDetacherCloser = new
	return modelcmd.Wrap(command)
}

const (
	detachStorageCommandDoc = `
Detaches storage from units. Specify one or more storage IDs (storage_name/id),
as output by ` + "`juju storage`" + `. The storage will remain in the model
until it is removed by an operator. The storage being detached will be removed
from all units that are using it.

Detaching storage may fail but under some circumstances, Juju user may need
to force storage detachment despite operational errors. Storage detachments are
not performed as a single operation, so when detaching multiple storage IDs it
may be that some detachments succeed while others fail. In this case the command
can be executed again to retry the failed detachments.
`

	detachStorageCommandExamples = `
    juju detach-storage pgdata/0
    juju detach-storage --force pgdata/0

`

	detachStorageCommandArgs = `<storage> [<storage> ...]`
)

// detachStorageCommand detaches storage instances.
type detachStorageCommand struct {
	StorageCommandBase
	modelcmd.IAASOnlyCommand
	newEntityDetacherCloser NewEntityDetacherCloserFunc
	storageIds              []string

	Force  bool
	NoWait bool
	fs     *gnuflag.FlagSet
}

// Init implements Command.Init.
func (c *detachStorageCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("detach-storage requires at least one storage ID")
	}
	c.storageIds = args
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *detachStorageCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Forcefully detach storage")
	c.fs = f
}

// Info implements Command.Info.
func (c *detachStorageCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "detach-storage",
		Purpose:  "Detaches storage from units.",
		Doc:      detachStorageCommandDoc,
		Examples: detachStorageCommandExamples,
		Args:     detachStorageCommandArgs,
		SeeAlso: []string{
			"storage",
			"attach-storage",
		},
	})
}

// Run implements Command.Run.
func (c *detachStorageCommand) Run(ctx *cmd.Context) error {
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
	if c.Force {
		if c.NoWait {
			zeroSec := 0 * time.Second
			maxWait = &zeroSec
		}
	}

	detacher, err := c.newEntityDetacherCloser(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer detacher.Close()

	results, err := detacher.Detach(ctx, c.storageIds, &c.Force, maxWait)
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			common.PermissionsMessage(ctx.Stderr, "detach storage")
		}
		return err
	}
	for i, result := range results {
		if result.Error == nil {
			ctx.Infof("detaching %s", c.storageIds[i])
		}
	}
	anyFailed := false
	for i, result := range results {
		if result.Error != nil {
			ctx.Infof("failed to detach %s: %s", c.storageIds[i], result.Error)
			anyFailed = true
		}
	}
	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}

// NewEntityDetacherCloser is the type of a function that returns an
// EntityDetacherCloser.
type NewEntityDetacherCloserFunc func(ctx context.Context) (EntityDetacherCloser, error)

// EntityDetacherCloser extends EntityDetacher with a Closer method.
type EntityDetacherCloser interface {
	EntityDetacher
	Close() error
}

// EntityDetacher defines an interface for detaching storage with the
// specified IDs.
type EntityDetacher interface {
	Detach(context.Context, []string, *bool, *time.Duration) ([]params.ErrorResult, error)
}
