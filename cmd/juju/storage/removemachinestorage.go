// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveVolumeCommandWithAPI returns a command used to remove a volume.
func NewRemoveVolumeCommandWithAPI() cmd.Command {
	cmd := &removeVolumeCommand{}
	cmd.newEntityDestroyerCloser = func() (EntityDestroyerCloser, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// NewRemoveFilesystemCommandWithAPI returns a command used to remove a filesystem.
func NewRemoveFilesystemCommandWithAPI() cmd.Command {
	cmd := &removeFilesystemCommand{}
	cmd.newEntityDestroyerCloser = func() (EntityDestroyerCloser, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// NewRemoveVolumeCommand returns a command used to remove a volume.
func NewRemoveVolumeCommand(new NewEntityDestroyerCloserFunc) cmd.Command {
	cmd := &removeVolumeCommand{}
	cmd.newEntityDestroyerCloser = new
	return modelcmd.Wrap(cmd)
}

// NewRemoveFilesystemCommand returns a command used to remove a filesystem.
func NewRemoveFilesystemCommand(new NewEntityDestroyerCloserFunc) cmd.Command {
	cmd := &removeFilesystemCommand{}
	cmd.newEntityDestroyerCloser = new
	return modelcmd.Wrap(cmd)
}

const (
	removeVolumeCommandDoc = `
Removes volumes from the model. Specify one or more volume IDs, as output by
"juju storage --volume".
`
	removeVolumeCommandArgs = `<volume ID> [<volume ID> ...]`

	removeFilesystemCommandDoc = `
Removes filesystems from the model. Specify one or more filesystem IDs, as
output by "juju storage --filesystem".
`
	removeFilesystemCommandArgs = `<filesystem ID> [<filesystem ID> ...]`
)

type removeMachineStorageCommandBase struct {
	StorageCommandBase
	newEntityDestroyerCloser NewEntityDestroyerCloserFunc
	tags                     []names.Tag
}

// Run implements Command.Run.
func (c *removeMachineStorageCommandBase) Run(ctx *cmd.Context) error {
	destroyer, err := c.newEntityDestroyerCloser()
	if err != nil {
		return errors.Trace(err)
	}
	defer destroyer.Close()

	results, err := destroyer.Destroy(c.tags)
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			common.PermissionsMessage(ctx.Stderr, "remove storage")
		}
		return err
	}
	return params.ErrorResults{results}.Combine()
}

// removeVolumeCommand removes volumes from the model.
type removeVolumeCommand struct {
	removeMachineStorageCommandBase
}

// Init implements Command.Init.
func (c *removeVolumeCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("remove-volume requires a volume ID")
	}
	tags := make([]names.Tag, len(args))
	for i, id := range args {
		if !names.IsValidVolume(id) {
			return errors.NotValidf("volume ID %q", id)
		}
		tags[i] = names.NewVolumeTag(id)
	}
	c.tags = tags
	return nil
}

// Info implements Command.Info.
func (c *removeVolumeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-volume",
		Purpose: "Removes volumes from the model.",
		Doc:     removeVolumeCommandDoc,
		Args:    removeVolumeCommandArgs,
	}
}

// removeFilesystemCommand removes filesystems from the model.
type removeFilesystemCommand struct {
	removeMachineStorageCommandBase
}

// Init implements Command.Init.
func (c *removeFilesystemCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("remove-filesystem requires a filesystem ID")
	}
	tags := make([]names.Tag, len(args))
	for i, id := range args {
		if !names.IsValidFilesystem(id) {
			return errors.NotValidf("filesystem ID %q", id)
		}
		tags[i] = names.NewFilesystemTag(id)
	}
	c.tags = tags
	return nil
}

// Info implements Command.Info.
func (c *removeFilesystemCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-filesystem",
		Purpose: "Removes filesystems from the model.",
		Doc:     removeFilesystemCommandDoc,
		Args:    removeFilesystemCommandArgs,
	}
}

// NewEntityDestroyerCloser is the type of a function that returns an
// EntityDestroyerCloser.
type NewEntityDestroyerCloserFunc func() (EntityDestroyerCloser, error)

// EntityDestroyerCloser extends EntityDestroyer with a Closer method.
type EntityDestroyerCloser interface {
	EntityDestroyer
	Close() error
}

// EntityDestroyer defines an interface for destroying storage entities
// with the specified tags.
type EntityDestroyer interface {
	Destroy([]names.Tag) ([]params.ErrorResult, error)
}
