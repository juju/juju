// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// NewDetachStorageCommandWithAPI returns a command
// used to detach storage from application units.
func NewDetachStorageCommandWithAPI() cmd.Command {
	command := &detachStorageCommand{}
	command.newEntityDetacherCloser = func() (EntityDetacherCloser, error) {
		return command.NewStorageAPI()
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
Detaches storage from units. Specify one or more unit/application storage IDs,
as output by ` + "`juju storage`" + `. The storage will remain in the model until it is
removed by an operator.

Detaching storage may fail but under some circumstances, Juju user may need
to force storage detachment despite operational errors.
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

	detacher, err := c.newEntityDetacherCloser()
	if err != nil {
		return errors.Trace(err)
	}
	defer detacher.Close()

	results, err := detacher.Detach(c.storageIds, &c.Force, maxWait)
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
type NewEntityDetacherCloserFunc func() (EntityDetacherCloser, error)

// EntityDetacherCloser extends EntityDetacher with a Closer method.
type EntityDetacherCloser interface {
	EntityDetacher
	Close() error
}

// EntityDetacher defines an interface for detaching storage with the
// specified IDs.
type EntityDetacher interface {
	Detach([]string, *bool, *time.Duration) ([]params.ErrorResult, error)
}
