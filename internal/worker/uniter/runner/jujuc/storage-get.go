// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
)

// StorageGetCommand implements the storage-get command.
type StorageGetCommand struct {
	cmd.CommandBase
	ctx             Context
	storageTag      names.StorageTag
	storageTagProxy gnuflag.Value
	key             string
	out             cmd.Output
}

func NewStorageGetCommand(cmdCtx Context) (cmd.Command, error) {
	c := &StorageGetCommand{ctx: cmdCtx}
	sV, err := newStorageIdValue(context.Background(), cmdCtx, &c.storageTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.storageTagProxy = sV
	return c, nil
}

func (c *StorageGetCommand) Info() *cmd.Info {
	doc := `
Inspect an available storage attachment of this unit. In a storage hook,
the default is the instance for that hook. Otherwise supply its Juju storage
ID with -s, using storage-list to find the IDs for this unit.

The optional key is kind or location; omit it to print both. A location is
a mount point for a filesystem or a device path for block storage.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/
`
	examples := `
Read the device path or mount point of the instance that triggered the current storage hook:

    storage-get location

Read the kind of storage instance data/0 after finding it with storage-list on this unit:

    storage-get -s data/0 kind
`
	return jujucmd.Info(&cmd.Info{
		Name:     "storage-get",
		Args:     "[-s <storage-id>] [<key>]",
		Purpose:  "Prints information for the storage instance with the specified ID.",
		Doc:      doc,
		Examples: examples,
	})
}

func (c *StorageGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
	f.Var(c.storageTagProxy, "s", "Specifies a storage instance by ID.")
}

func (c *StorageGetCommand) Init(args []string) error {
	if c.storageTag == (names.StorageTag{}) {
		return errors.New("no storage instance specified")
	}
	key, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	c.key = key
	return nil
}

func (c *StorageGetCommand) Run(ctx *cmd.Context) error {
	storage, err := c.ctx.Storage(ctx, c.storageTag)
	if err != nil {
		return errors.Trace(err)
	}
	values := map[string]any{
		"kind":     storage.Kind().String(),
		"location": storage.Location(),
	}
	if c.key == "" {
		return c.out.Write(ctx, values)
	}
	if value, ok := values[c.key]; ok {
		return c.out.Write(ctx, value)
	}
	return errors.Errorf("invalid storage attribute %q", c.key)
}
