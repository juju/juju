// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

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

func NewStorageGetCommand(ctx Context) (cmd.Command, error) {
	c := &StorageGetCommand{ctx: ctx}
	sV, err := newStorageIdValue(ctx, &c.storageTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.storageTagProxy = sV
	return c, nil
}

func (c *StorageGetCommand) Info() *cmd.Info {
	doc := `
When no <key> is supplied, all keys values are printed.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "storage-get",
		Args:    "[<key>]",
		Purpose: "print information for storage instance with specified id",
		Doc:     doc,
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
	storage, err := c.ctx.Storage(c.storageTag)
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
