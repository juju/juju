// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

// StorageGetCommand implements the storage-get command.
type StorageGetCommand struct {
	cmd.CommandBase
	ctx               Context
	storageInstanceId string
	keys              []string
	out               cmd.Output
}

func NewStorageGetCommand(ctx Context) cmd.Command {
	return &StorageGetCommand{ctx: ctx}
}

func (c *StorageGetCommand) Info() *cmd.Info {
	doc := `
When no <key> is supplied, all keys values are printed.
`
	return &cmd.Info{
		Name:    "storage-get",
		Args:    "<storageInstanceId> <key> [<key>]*",
		Purpose: "print information for storage instance with specified id",
		Doc:     doc,
	}
}

func (c *StorageGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *StorageGetCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no storage instance specified")
	}
	if len(args) < 2 {
		return errors.New("no attribute keys specified")
	}
	c.storageInstanceId = args[0]
	c.keys = args[1:]
	return nil
}

func (c *StorageGetCommand) Run(ctx *cmd.Context) error {
	storageInstance, ok := c.ctx.StorageInstance(c.storageInstanceId)
	if !ok {
		return nil
	}
	values := make(map[string]interface{})
	var singleValue interface{}
	for _, key := range c.keys {
		switch key {
		case "kind":
			values[key] = storageInstance.Kind
		case "location":
			values[key] = storageInstance.Location
		default:
			return errors.Errorf("invalid storage instance key %q", key)
		}
		singleValue = values[key]
	}
	// For single values with smart formatting, we want just the value printed,
	// not "key: value".
	if len(c.keys) == 1 && c.out.Name() == "smart" {
		return c.out.Write(ctx, singleValue)
	}
	return c.out.Write(ctx, values)
}
