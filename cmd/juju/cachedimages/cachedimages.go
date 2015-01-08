// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cachedimages

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/api/imagemanager"
	"github.com/juju/juju/cmd/envcmd"
)

const cachedimagesCommandDoc = `
"juju cached-images" is used to manage the cached os images in
the Juju environment.
`

const cachedImagesCommandPurpose = "manage cached os images"

// NewSuperCommand creates the user supercommand and registers the subcommands
// that it supports.
func NewSuperCommand() cmd.Command {
	usercmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "cached-images",
		Doc:         cachedimagesCommandDoc,
		UsagePrefix: "juju",
		Purpose:     cachedImagesCommandPurpose,
	})
	usercmd.Register(envcmd.Wrap(&DeleteCommand{}))
	usercmd.Register(envcmd.Wrap(&ListCommand{}))
	return usercmd
}

// CachedImagesCommandBase is a helper base structure that has a method to get the
// image manager client.
type CachedImagesCommandBase struct {
	envcmd.EnvCommandBase
}

// NewImagesManagerClient returns a imagemanager client for the root api endpoint
// that the environment command returns.
func (c *CachedImagesCommandBase) NewImagesManagerClient() (*imagemanager.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return imagemanager.NewClient(root), nil
}
