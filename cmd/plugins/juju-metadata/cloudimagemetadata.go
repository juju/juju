// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/imagemetadata"
)

type CloudImageMetadataCommandBase struct {
	ImageMetadataCommandBase
}

// SetFlags implements Command.SetFlags.
func (c *CloudImageMetadataCommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.ImageMetadataCommandBase.SetFlags(f)
}

// NewImageMetadataAPI returns a image metadata api for the root api endpoint
// that the environment command returns.
func (c *CloudImageMetadataCommandBase) NewImageMetadataAPI() (*imagemetadata.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return imagemetadata.NewClient(root), nil
}

// MetadataInfo defines the serialization behaviour of image metadata information.
type MetadataInfo struct {
	Source          string `yaml:"source" json:"source"`
	Series          string `yaml:"series" json:"series"`
	Arch            string `yaml:"arch" json:"arch"`
	Region          string `yaml:"region" json:"region"`
	ImageId         string `yaml:"image_id" json:"image_id"`
	Stream          string `yaml:"stream" json:"stream"`
	VirtType        string `yaml:"virt_type" json:"virt_type"`
	RootStorageType string `yaml:"storage_type" json:"storage_type"`
}
