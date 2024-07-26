// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

func newDeleteImageMetadataCommand() cmd.Command {
	deleteCmd := &deleteImageMetadataCommand{}
	deleteCmd.newAPIFunc = func(ctx context.Context) (MetadataDeleteAPI, error) {
		return deleteCmd.NewImageMetadataAPI(ctx)
	}
	return modelcmd.Wrap(deleteCmd)
}

const deleteImageCommandDoc = `
Delete image metadata from Juju environment.
`

// deleteImageMetadataCommand deletes image metadata from Juju environment.
type deleteImageMetadataCommand struct {
	cloudImageMetadataCommandBase

	newAPIFunc func(ctx context.Context) (MetadataDeleteAPI, error)

	ImageId string
}

// Init implements Command.Init.
func (c *deleteImageMetadataCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("image ID must be supplied when deleting image metadata")
	}
	if len(args) != 1 {
		return errors.New("only one image ID can be supplied as an argument to this command")
	}
	c.ImageId = args[0]
	return nil
}

// Info implements Command.Info.
func (c *deleteImageMetadataCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "delete-image",
		Purpose: "deletes image metadata from environment",
		Doc:     deleteImageCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *deleteImageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.cloudImageMetadataCommandBase.SetFlags(f)
}

// Run implements Command.Run.
func (c *deleteImageMetadataCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc(ctx)
	if err != nil {
		return err
	}
	defer api.Close()

	err = api.Delete(ctx, c.ImageId)
	if err != nil {
		return err
	}
	return nil
}

// MetadataDeleteAPI defines the API methods that delete image metadata command uses.
type MetadataDeleteAPI interface {
	Close() error
	Delete(ctx context.Context, imageId string) error
}
