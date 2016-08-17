// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package cachedimages

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

const removeCommandDoc = `
Remove cached os images in the Juju model.

Images are identified by:
  Kind         eg "lxd"
  Series       eg "xenial"
  Architecture eg "amd64"

Examples:
  # Remove cached lxd image for xenial amd64.
  juju remove-cached-images --kind lxd --series xenial --arch amd64
`

// NewRemoveCommand returns a command used to remove cached images.
func NewRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

// removeCommand shows the images in the Juju server.
type removeCommand struct {
	CachedImagesCommandBase
	Kind, Series, Arch string
}

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-cached-images",
		Purpose: "Remove cached OS images.",
		Doc:     removeCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CachedImagesCommandBase.SetFlags(f)
	f.StringVar(&c.Kind, "kind", "", "The image kind to remove eg lxd")
	f.StringVar(&c.Series, "series", "", "The series of the image to remove eg xenial")
	f.StringVar(&c.Arch, "arch", "", "The architecture of the image to remove eg amd64")
}

// Init implements Command.Init.
func (c *removeCommand) Init(args []string) (err error) {
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

// RemoveImageAPI defines the imagemanager API methods that the remove command uses.
type RemoveImageAPI interface {
	DeleteImage(kind, series, arch string) error
	Close() error
}

var getRemoveImageAPI = func(p *CachedImagesCommandBase) (RemoveImageAPI, error) {
	return p.NewImagesManagerClient()
}

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) (err error) {
	client, err := getRemoveImageAPI(&c.CachedImagesCommandBase)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.DeleteImage(c.Kind, c.Series, c.Arch)
}
