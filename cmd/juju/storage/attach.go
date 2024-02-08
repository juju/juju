// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// NewAttachStorageCommandWithAPI returns a command
// used to attach storage to application units.
func NewAttachStorageCommandWithAPI() cmd.Command {
	cmd := &attachStorageCommand{}
	cmd.newEntityAttacherCloser = func() (EntityAttacherCloser, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// NewAttachStorageCommand returns a command used to
// attach storage to application units.
func NewAttachStorageCommand(new NewEntityAttacherCloserFunc) cmd.Command {
	cmd := &attachStorageCommand{}
	cmd.newEntityAttacherCloser = new
	return modelcmd.Wrap(cmd)
}

const (
	attachStorageCommandDoc = `
Attach existing storage to a unit. Specify a unit
and one or more storage IDs to attach to it.
`
	attachStorageCommandExamples = `
    juju attach-storage postgresql/1 pgdata/0

`
	attachStorageCommandArgs = `<unit> <storage> [<storage> ...]`
)

// attachStorageCommand adds unit storage instances dynamically.
type attachStorageCommand struct {
	StorageCommandBase
	modelcmd.IAASOnlyCommand
	newEntityAttacherCloser NewEntityAttacherCloserFunc
	unitId                  string
	storageIds              []string
}

// Init implements Command.Init.
func (c *attachStorageCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("attach-storage requires a unit ID and at least one storage ID")
	}
	c.unitId = args[0]
	c.storageIds = args[1:]
	return nil
}

// Info implements Command.Info.
func (c *attachStorageCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "attach-storage",
		Purpose:  "Attaches existing storage to a unit.",
		Doc:      attachStorageCommandDoc,
		Args:     attachStorageCommandArgs,
		Examples: attachStorageCommandExamples,
	})
}

// Run implements Command.Run.
func (c *attachStorageCommand) Run(ctx *cmd.Context) error {
	attacher, err := c.newEntityAttacherCloser()
	if err != nil {
		return err
	}
	defer attacher.Close()

	results, err := attacher.Attach(c.unitId, c.storageIds)
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			common.PermissionsMessage(ctx.Stderr, "attach storage")
		}
		return block.ProcessBlockedError(errors.Annotatef(err, "could not attach storage %v", c.storageIds), block.BlockChange)
	}
	for i, result := range results {
		if result.Error == nil {
			ctx.Infof("attaching %s to %s", c.storageIds[i], c.unitId)
		}
	}
	var anyFailed bool
	for i, result := range results {
		if result.Error != nil {
			ctx.Infof("failed to attach %s to %s: %s", c.storageIds[i], c.unitId, result.Error)
			anyFailed = true
		}
	}
	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}

// NewEntityAttacherCloser is the type of a function that returns an
// EntityAttacherCloser.
type NewEntityAttacherCloserFunc func() (EntityAttacherCloser, error)

// EntityAttacherCloser extends EntityAttacher with a Closer method.
type EntityAttacherCloser interface {
	EntityAttacher
	Close() error
}

// EntityAttacher defines an interface for attaching storage with the
// specified IDs to a unit.
type EntityAttacher interface {
	Attach(string, []string) ([]params.ErrorResult, error)
}
