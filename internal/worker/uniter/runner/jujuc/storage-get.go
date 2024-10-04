// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"context"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	jujucmd "github.com/juju/juju/cmd"
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
	sV, err := newStorageIdValue(context.TODO(), cmdCtx, &c.storageTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.storageTagProxy = sV
	return c, nil
}

func (c *StorageGetCommand) Info() *cmd.Info {
	doc := `
When no <key> is supplied, all keys values are printed.

Further details:
storage-get obtains information about storage being attached
to, or detaching from, the unit.

If the executing hook is a storage hook, information about
the storage related to the hook will be reported; this may
be overridden by specifying the name of the storage as reported
by storage-list, and must be specified for non-storage hooks.

storage-get can be used to identify the storage location during
storage-attached and storage-detaching hooks. The exception to
this is when the charm specifies a static location for
singleton stores.
`
	examples := `
    # retrieve information by UUID
    storage-get 21127934-8986-11e5-af63-feff819cdc9f

    # retrieve information by name
    storage-get -s data/0
`
	return jujucmd.Info(&cmd.Info{
		Name:     "storage-get",
		Args:     "[<key>]",
		Purpose:  "Print information for the storage instance with the specified ID.",
		Doc:      doc,
		Examples: examples,
	})
}

func (c *StorageGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
	f.Var(c.storageTagProxy, "s", "specify a storage instance by id")
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
	values := map[string]interface{}{
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
