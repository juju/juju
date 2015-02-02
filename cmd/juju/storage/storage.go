// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.storage")

const storageCmdDoc = `
"juju storage" is used to manage storage instances in
 the Juju environment.
`

const storageCmdPurpose = "manage storage instances"

// Command is the top-level command wrapping all storage functionality.
type Command struct {
	cmd.SuperCommand
}

// NewSuperCommand creates the storage supercommand and
// registers the subcommands that it supports.
func NewSuperCommand() cmd.Command {
	storagecmd := Command{
		SuperCommand: *cmd.NewSuperCommand(
			cmd.SuperCommandParams{
				Name:        "storage",
				Doc:         storageCmdDoc,
				UsagePrefix: "juju",
				Purpose:     storageCmdPurpose,
			})}
	storagecmd.Register(envcmd.Wrap(&ShowCommand{}))
	return &storagecmd
}

// StorageCommandBase is a helper base structure that has a method to get the
// storage managing client.
type StorageCommandBase struct {
	envcmd.EnvCommandBase
}

// NewStorageAPI returns a storage api for the root api endpoint
// that the environment command returns.
func (c *StorageCommandBase) NewStorageAPI() (*storage.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return storage.NewClient(root), nil
}
