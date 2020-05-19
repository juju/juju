// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/os/series"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

func newAddImageMetadataCommand() cmd.Command {
	return modelcmd.Wrap(&addImageMetadataCommand{})
}

const addImageCommandDoc = `
Add image metadata to Juju model.

Image metadata properties vary between providers. Consequently, some properties
are optional for this command but they may still be needed by your provider.

This command takes only one positional argument - an image id.

arguments:
image-id
   image identifier

options:
-m, --model (= "")
   juju model to operate in
--region
   cloud region (= region of current model)
--series (= current model preferred series)
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
	if len(args) == 0 {
		return errors.New("image id must be supplied when adding image metadata")
	}
	if len(args) != 1 {
		return errors.New("only one image id can be supplied as an argument to this command")
	}
	c.ImageId = args[0]
	return c.validate()
}

// Info implements Command.Info.
func (c *addImageMetadataCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add-image",
		Purpose: "adds image metadata to model",
		Doc:     addImageCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *addImageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.cloudImageMetadataCommandBase.SetFlags(f)

	f.StringVar(&c.Region, "region", "", "image cloud region")
	f.StringVar(&c.Series, "series", "", "image series")
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
	if err := api.Save([]params.CloudImageMetadata{m}); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// MetadataAddAPI defines the API methods that add image metadata command uses.
type MetadataAddAPI interface {
	Close() error
	Save(metadata []params.CloudImageMetadata) error
}

var getImageMetadataAddAPI = (*addImageMetadataCommand).getImageMetadataAddAPI

func (c *addImageMetadataCommand) getImageMetadataAddAPI() (MetadataAddAPI, error) {
	return c.NewImageMetadataAPI()
}

// Init implements Command.Init.
func (c *addImageMetadataCommand) validate() error {
	if c.Series != "" {
		if _, err := series.SeriesVersion(c.Series); err != nil {
			return errors.Trace(err)
		}
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
		Priority:        simplestreams.CUSTOM_CLOUD_DATA,
	}
	if c.RootStorageSize != 0 {
		info.RootStorageSize = &c.RootStorageSize
	}
	return info
}
