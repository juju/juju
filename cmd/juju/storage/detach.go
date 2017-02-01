// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewDetachStorageCommandWithAPI returns a command used to detach storage
// instances.
func NewDetachStorageCommandWithAPI() cmd.Command {
	cmd := &detachStorageCommand{}
	cmd.newEntityDestroyerCloser = func() (EntityDestroyerCloser, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// NewDetachStorageCommand returns a command used to detach storage instances.
func NewDetachStorageCommand(new NewEntityDestroyerCloserFunc) cmd.Command {
	cmd := &detachStorageCommand{}
	cmd.newEntityDestroyerCloser = new
	return modelcmd.Wrap(cmd)
}

const (
	detachStorageCommandDoc = `
Detaches storage from units/applications. Specify one or more storage IDs,
as output by "juju storage".
`
	detachStorageCommandArgs = `<storage ID> [<storage ID> ...]`
)

// detachStorageCommand detaches storage instances.
type detachStorageCommand struct {
	removeStorageCommandBase
}

// Init implements Command.Init.
func (c *detachStorageCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("remove-storage requires a storage ID")
	}
	tags := make([]names.Tag, len(args))
	for i, id := range args {
		if !names.IsValidStorage(id) {
			return errors.NotValidf("storage ID %q", id)
		}
		tags[i] = names.NewStorageTag(id)
	}
	c.tags = tags
	return nil
}

// Info implements Command.Info.
func (c *detachStorageCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "detach-storage",
		Purpose: "Detaches storage from units/applications.",
		Doc:     detachStorageCommandDoc,
		Args:    detachStorageCommandArgs,
	}
}
