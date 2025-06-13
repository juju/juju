// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v3/arch"

	"github.com/juju/juju/caas"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/version"
)

func prepare(ctx *cmd.Context, controllerName string, store jujuclient.ClientStore) (environs.Environ, error) {
	// NOTE(axw) this is a work-around for the TODO below. This
	// means that the command will only work if you've bootstrapped
	// the specified environment.
	bootstrapConfig, params, err := modelcmd.NewGetBootstrapConfigParamsFunc(
		ctx, store, environs.GlobalProviderRegistry(),
	)(controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := environs.Provider(bootstrapConfig.CloudType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, ok := provider.(caas.ContainerEnvironProvider); ok {
		return nil, errors.NotSupportedf("preparing environ for CAAS")
	}
	cfg, err := provider.PrepareConfig(*params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(axw) we'll need to revise the metadata commands to work
	// without preparing an environment. They should take the same
	// format as bootstrap, i.e. cloud/region, and we'll use that to
	// identify region and endpoint info that we need. Not sure what
	// we'll do about simplestreams.MetadataValidator yet. Probably
	// move it to the EnvironProvider interface.
	return environs.New(context.TODO(), environs.OpenParams{
		Cloud:  params.Cloud,
		Config: cfg,
	})
}

func newImageMetadataCommand() cmd.Command {
	return modelcmd.WrapController(&imageMetadataCommand{})
}

// imageMetadataCommand is used to write out simplestreams image metadata information.
type imageMetadataCommand struct {
	modelcmd.ControllerCommandBase

	Dir            string
	Series         string
	Arch           string
	ImageId        string
	Region         string
	Endpoint       string
	Stream         string
	VirtType       string
	Storage        string
	privateStorage string
}

var imageMetadataDoc = `
generate-image creates simplestreams image metadata for the specified cloud.

The cloud specification comes from the current Juju model, as specified in
the usual way from either the -m option, or JUJU_MODEL.

Using command arguments, it is possible to override cloud attributes region, endpoint, and series.
By default, "amd64" is used for the architecture but this may also be changed.
`

func (c *imageMetadataCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "generate-image",
		Purpose: "generate simplestreams image metadata",
		Doc:     imageMetadataDoc,
	})
}

func (c *imageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Series, "s", "", "the charm series")
	f.StringVar(&c.Arch, "a", arch.AMD64, "the image achitecture")
	f.StringVar(&c.Dir, "d", "", "the destination directory in which to place the metadata files")
	f.StringVar(&c.ImageId, "i", "", "the image id")
	f.StringVar(&c.Region, "r", "", "the region")
	f.StringVar(&c.Endpoint, "u", "", "the cloud endpoint (for Openstack, this is the Identity Service endpoint)")
	f.StringVar(&c.Stream, "stream", imagemetadata.ReleasedStream, "the image stream")
	f.StringVar(&c.VirtType, "virt-type", "", "the image virtualisation type")
	f.StringVar(&c.Storage, "storage", "", "the type of root storage")
}

// setParams sets parameters based on the environment configuration
// for those which have not been explicitly specified.
func (c *imageMetadataCommand) setParams(context *cmd.Context) error {
	c.privateStorage = "<private storage name>"

	controllerName, err := c.ControllerName()
	err = errors.Cause(err)
	if err != nil && err != modelcmd.ErrNoControllersDefined && err != modelcmd.ErrNoCurrentController {
		return errors.Trace(err)
	}

	var environ environs.Environ
	if err == nil {
		if environ, err := prepare(context, controllerName, c.ClientStore()); err == nil {
			logger.Infof("creating image metadata for model %q", environ.Config().Name())
			// If the user has not specified region and endpoint, try and get it from the environment.
			if c.Region == "" || c.Endpoint == "" {
				var cloudSpec simplestreams.CloudSpec
				if inst, ok := environ.(simplestreams.HasRegion); ok {
					if cloudSpec, err = inst.Region(); err != nil {
						return err
					}
				} else {
					return errors.Errorf("model %q cannot provide region and endpoint", environ.Config().Name())
				}
				// If only one of region or endpoint is provided, that is a problem.
				if cloudSpec.Region != cloudSpec.Endpoint && (cloudSpec.Region == "" || cloudSpec.Endpoint == "") {
					return errors.Errorf("cannot generate metadata without a complete cloud configuration")
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
		} else {
			logger.Warningf("bootstrap parameters could not be opened: %v", err)
		}
	}
	if environ == nil {
		logger.Infof("no model found, creating image metadata using user supplied data")
	}
	if c.Series == "" {
		c.Series = version.DefaultSupportedLTS()
	}
	if c.ImageId == "" {
		return errors.Errorf("image id must be specified")
	}
	if c.Region == "" {
		return errors.Errorf("image region must be specified")
	}
	if c.Endpoint == "" {
		return errors.Errorf("cloud endpoint URL must be specified")
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
Image metadata files have been written to:
%s.
For Juju to use this metadata, the files need to be put into the
image metadata search path. There are 2 options:

1. For local access, use the --metadata-source parameter when bootstrapping:
   juju bootstrap --metadata-source %s [...]

2. For remote access, use image-metadata-url attribute for model configuration.
To set it as a default for any model or for the controller model,
it needs to be supplied as part of --model-default to 'juju bootstrap' command.
See 'bootstrap' help for more details.
For configuration for a particular model, set it as --image-metadata-url on
'juju model-config'. See 'model-config' help for more details.
Regardless of where this attribute is used, it expects a reachable URL.
You need to configure a http server to serve the contents of
%s
and set the value of image-metadata-url accordingly.
`

func (c *imageMetadataCommand) Run(context *cmd.Context) error {
	if err := c.setParams(context); err != nil {
		return err
	}
	out := context.Stdout
	im := &imagemetadata.ImageMetadata{
		Id:       c.ImageId,
		Arch:     c.Arch,
		Stream:   c.Stream,
		VirtType: c.VirtType,
		Storage:  c.Storage,
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   c.Region,
		Endpoint: c.Endpoint,
	}
	targetStorage, err := filestorage.NewFileStorageWriter(c.Dir)
	if err != nil {
		return err
	}
	fetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	err = imagemetadata.MergeAndWriteMetadata(fetcher, c.Series, []*imagemetadata.ImageMetadata{im}, &cloudSpec, targetStorage)
	if err != nil {
		return errors.Errorf("image metadata files could not be created: %v", err)
	}
	dir := context.AbsPath(c.Dir)
	dest := filepath.Join(dir, storage.BaseImagesPath, "streams", "v1")
	fmt.Fprintf(out, helpDoc, dest, dir, dir)
	return nil
}
