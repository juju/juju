// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/juju/arch"
)

type ImageMetadataCommandBase struct {
	envcmd.EnvCommandBase
}

func (c *ImageMetadataCommandBase) prepare(context *cmd.Context, store configstore.Storage) (environs.Environ, error) {
	cfg, err := c.Config(store)
	if err != nil {
		return nil, errors.Annotate(err, "could not get config from store")
	}
	return environs.Prepare(cfg, context, store)
}

// ImageMetadataCommand is used to write out simplestreams image metadata information.
type ImageMetadataCommand struct {
	ImageMetadataCommandBase
	Dir            string
	Series         string
	Arch           string
	ImageId        string
	Region         string
	Endpoint       string
	privateStorage string
}

var imageMetadataDoc = `
generate-image creates simplestreams image metadata for the specified cloud.

The cloud specification comes from the current Juju environment, as specified in
the usual way from either ~/.juju/environments.yaml, the -e option, or JUJU_ENV.

Using command arguments, it is possible to override cloud attributes region, endpoint, and series.
By default, "amd64" is used for the architecture but this may also be changed.
`

func (c *ImageMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "generate-image",
		Purpose: "generate simplestreams image metadata",
		Doc:     imageMetadataDoc,
	}
}

func (c *ImageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Series, "s", "", "the charm series")
	f.StringVar(&c.Arch, "a", arch.AMD64, "the image achitecture")
	f.StringVar(&c.Dir, "d", "", "the destination directory in which to place the metadata files")
	f.StringVar(&c.ImageId, "i", "", "the image id")
	f.StringVar(&c.Region, "r", "", "the region")
	f.StringVar(&c.Endpoint, "u", "", "the cloud endpoint (for Openstack, this is the Identity Service endpoint)")
}

// setParams sets parameters based on the environment configuration
// for those which have not been explicitly specified.
func (c *ImageMetadataCommand) setParams(context *cmd.Context) error {
	c.privateStorage = "<private storage name>"
	var environ environs.Environ
	if store, err := configstore.Default(); err == nil {
		if environ, err = c.prepare(context, store); err == nil {
			logger.Infof("creating image metadata for environment %q", environ.Name())
			// If the user has not specified region and endpoint, try and get it from the environment.
			if c.Region == "" || c.Endpoint == "" {
				var cloudSpec simplestreams.CloudSpec
				if inst, ok := environ.(simplestreams.HasRegion); ok {
					if cloudSpec, err = inst.Region(); err != nil {
						return err
					}
				} else {
					return fmt.Errorf("environment %q cannot provide region and endpoint", environ.Name())
				}
				// If only one of region or endpoint is provided, that is a problem.
				if cloudSpec.Region != cloudSpec.Endpoint && (cloudSpec.Region == "" || cloudSpec.Endpoint == "") {
					return fmt.Errorf("cannot generate metadata without a complete cloud configuration")
				}
				if c.Region == "" {
					c.Region = cloudSpec.Region
				}
				if c.Endpoint == "" {
					c.Endpoint = cloudSpec.Endpoint
				}
			}
			cfg := environ.Config()
			if c.Series == "" {
				c.Series = config.PreferredSeries(cfg)
			}
			if v, ok := cfg.AllAttrs()["control-bucket"]; ok {
				c.privateStorage = v.(string)
			}
		} else {
			logger.Warningf("environment could not be opened: %v", err)
		}
	}
	if environ == nil {
		logger.Infof("no environment found, creating image metadata using user supplied data")
	}
	if c.Series == "" {
		c.Series = config.LatestLtsSeries()
	}
	if c.ImageId == "" {
		return fmt.Errorf("image id must be specified")
	}
	if c.Region == "" {
		return fmt.Errorf("image region must be specified")
	}
	if c.Endpoint == "" {
		return fmt.Errorf("cloud endpoint URL must be specified")
	}
	if c.Dir == "" {
		logger.Infof("no destination directory specified, using current directory")
		var err error
		if c.Dir, err = os.Getwd(); err != nil {
			return err
		}
	}
	return nil
}

var helpDoc = `
image metadata files have been written to:
%s.
For Juju to use this metadata, the files need to be put into the
image metadata search path. There are 2 options:

1. Use the --metadata-source parameter when bootstrapping:
   juju bootstrap --metadata-source %s

2. Use image-metadata-url in $JUJU_HOME/environments.yaml
Configure a http server to serve the contents of
%s
and set the value of image-metadata-url accordingly.

"

`

func (c *ImageMetadataCommand) Run(context *cmd.Context) error {
	if err := c.setParams(context); err != nil {
		return err
	}
	out := context.Stdout
	im := &imagemetadata.ImageMetadata{
		Id:   c.ImageId,
		Arch: c.Arch,
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   c.Region,
		Endpoint: c.Endpoint,
	}
	targetStorage, err := filestorage.NewFileStorageWriter(c.Dir)
	if err != nil {
		return err
	}
	err = imagemetadata.MergeAndWriteMetadata(c.Series, []*imagemetadata.ImageMetadata{im}, &cloudSpec, targetStorage)
	if err != nil {
		return fmt.Errorf("image metadata files could not be created: %v", err)
	}
	dir := context.AbsPath(c.Dir)
	dest := filepath.Join(dir, storage.BaseImagesPath, "streams", "v1")
	fmt.Fprintf(out, fmt.Sprintf(helpDoc, dest, dir, dir))
	return nil
}
