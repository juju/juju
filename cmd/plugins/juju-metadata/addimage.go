// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

func newAddImageMetadataCommand() cmd.Command {
	return envcmd.Wrap(&addImageMetadataCommand{})
}

const addImageCommandDoc = `
Add image metadata to Juju environment.

Image metadata properties vary between providers. Consequently, some properties
are optional for this command but they may still be needed by your provider.

options:
-e, --environment (= "")
   juju environment to operate in
--image-id
   image identifier
--region
   cloud region
--series (= "trusty")
   image series
--arch (= "amd64")
   image architectures
--virt-type
   virtualisation type [provider specific], e.g. hmv
--storage-type
   root storage type [provider specific], e.g. ebs
--storage-size
   root storage size [provider specific]
--stream (= "released")
   image stream
`

// addImageMetadataCommand stores image metadata in Juju environment.
type addImageMetadataCommand struct {
	cloudImageMetadataCommandBase

	ImageId         string
	Region          string
	Series          string
	Arch            string
	VirtType        string
	RootStorageType string
	RootStorageSize uint64
	Stream          string
}

// Init implements Command.Init.
func (c *addImageMetadataCommand) Init(args []string) (err error) {
	if err := checkArgumentSet(c.ImageId, "image id"); err != nil {
		return err
	}
	return nil
}

// Info implements Command.Info.
func (c *addImageMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-image",
		Purpose: "adds image metadata to environment",
		Doc:     addImageCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *addImageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.cloudImageMetadataCommandBase.SetFlags(f)

	f.StringVar(&c.ImageId, "image-id", "", "metadata image id")
	f.StringVar(&c.Region, "region", "", "image cloud region")
	// TODO (anastasiamac 2015-09-30) Ideally default should be latest LTS.
	// Hard-coding "trusty" for now.
	f.StringVar(&c.Series, "series", "trusty", "image series")
	f.StringVar(&c.Arch, "arch", "amd64", "image architecture")
	f.StringVar(&c.VirtType, "virt-type", "", "image metadata virtualisation type")
	f.StringVar(&c.RootStorageType, "storage-type", "", "image metadata root storage type")
	f.Uint64Var(&c.RootStorageSize, "storage-size", 0, "image metadata root storage size")
	f.StringVar(&c.Stream, "stream", "released", "image metadata stream")
}

// Run implements Command.Run.
func (c *addImageMetadataCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getImageMetadataAddAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	m := c.constructMetadataParam()
	found, err := api.Save([]params.CloudImageMetadata{m})
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return nil
	}
	if len(found) > 1 {
		return errors.New(fmt.Sprintf("expected one result, got %d", len(found)))
	}
	if found[0].Error != nil {
		return errors.New(found[0].Error.GoString())
	}
	return nil
}

// MetadataAddAPI defines the API methods that add image metadata command uses.
type MetadataAddAPI interface {
	Close() error
	Save(metadata []params.CloudImageMetadata) ([]params.ErrorResult, error)
}

var getImageMetadataAddAPI = (*addImageMetadataCommand).getImageMetadataAddAPI

func (c *addImageMetadataCommand) getImageMetadataAddAPI() (MetadataAddAPI, error) {
	return c.NewImageMetadataAPI()
}

func checkArgumentSet(arg, name string) (err error) {
	if arg == "" {
		return errors.New(fmt.Sprintf("%v must be supplied when adding an image metadata", name))
	}
	return nil
}

// constructMetadataParam returns cloud image metadata as a param.
func (c *addImageMetadataCommand) constructMetadataParam() params.CloudImageMetadata {
	info := params.CloudImageMetadata{
		ImageId:         c.ImageId,
		Region:          c.Region,
		Series:          c.Series,
		Arch:            c.Arch,
		VirtType:        c.VirtType,
		RootStorageType: c.RootStorageType,
		Stream:          c.Stream,
		Source:          "custom",
	}
	if c.RootStorageSize != 0 {
		info.RootStorageSize = &c.RootStorageSize
	}
	return info
}
