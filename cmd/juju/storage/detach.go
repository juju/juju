// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewDetachStorageCommandWithAPI returns a command
// used to detach storage from application units.
func NewDetachStorageCommandWithAPI() cmd.Command {
	cmd := &detachStorageCommand{}
	cmd.newEntityDetacherCloser = func() (EntityDetacherCloser, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// NewDetachStorageCommand returns a command used to
// detach storage from application units.
func NewDetachStorageCommand(new NewEntityDetacherCloserFunc) cmd.Command {
	cmd := &detachStorageCommand{}
	cmd.newEntityDetacherCloser = new
	return modelcmd.Wrap(cmd)
}

const (
	detachStorageCommandDoc = `
Detaches storage from units. Specify one or more unit/application storage IDs,
as output by "juju storage". The storage will remain in the model until it is
removed by an operator.

Examples:
    juju detach-storage pgdata/0
`

	detachStorageCommandArgs = `<storage> [<storage> ...]`
)

// detachStorageCommand detaches storage instances.
type detachStorageCommand struct {
	StorageCommandBase
	newEntityDetacherCloser NewEntityDetacherCloserFunc
	storageIds              []string
}

// Init implements Command.Init.
func (c *detachStorageCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("detach-storage requires at least one storage ID")
	}
	c.storageIds = args
	return nil
}

// Info implements Command.Info.
func (c *detachStorageCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "detach-storage",
		Purpose: "Detaches storage from units.",
		Doc:     detachStorageCommandDoc,
		Args:    detachStorageCommandArgs,
	}
}

// Run implements Command.Run.
func (c *detachStorageCommand) Run(ctx *cmd.Context) error {
	detacher, err := c.newEntityDetacherCloser()
	if err != nil {
		return errors.Trace(err)
	}
	defer detacher.Close()

	results, err := detacher.Detach(c.storageIds)
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
	Detach([]string) ([]params.ErrorResult, error)
}
