// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package cachedimages

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

const deleteCommandDoc = `
Delete cached os images in the Juju model.

Images are identified by:
  Kind         eg "lxc"
  Series       eg "trusty"
  Architecture eg "amd64"

Examples:

  # Delete cached lxc image for trusty amd64.
  juju cache-images delete --kind lxc --series trusty --arch amd64
`

func newDeleteCommand() cmd.Command {
	return modelcmd.Wrap(&deleteCommand{})
}

// deleteCommand shows the images in the Juju server.
type deleteCommand struct {
	CachedImagesCommandBase
	Kind, Series, Arch string
}

// Info implements Command.Info.
func (c *deleteCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "delete",
		Purpose: "delete cached os images",
		Doc:     deleteCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *deleteCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CachedImagesCommandBase.SetFlags(f)
	f.StringVar(&c.Kind, "kind", "", "the image kind to delete eg lxc")
	f.StringVar(&c.Series, "series", "", "the series of the image to delete eg trusty")
	f.StringVar(&c.Arch, "arch", "", "the architecture of the image to delete eg amd64")
}

// Init implements Command.Init.
func (c *deleteCommand) Init(args []string) (err error) {
	if c.Kind == "" {
		return errors.New("image kind must be specified")
	}
	if c.Series == "" {
		return errors.New("image series must be specified")
	}
	if c.Arch == "" {
		return errors.New("image architecture must be specified")
	}
	return cmd.CheckEmpty(args)
}

// DeleteImageAPI defines the imagemanager API methods that the delete command uses.
type DeleteImageAPI interface {
	DeleteImage(kind, series, arch string) error
	Close() error
}

var getDeleteImageAPI = func(p *CachedImagesCommandBase) (DeleteImageAPI, error) {
	return p.NewImagesManagerClient()
}

// Run implements Command.Run.
func (c *deleteCommand) Run(ctx *cmd.Context) (err error) {
	client, err := getDeleteImageAPI(&c.CachedImagesCommandBase)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.DeleteImage(c.Kind, c.Series, c.Arch)
}
