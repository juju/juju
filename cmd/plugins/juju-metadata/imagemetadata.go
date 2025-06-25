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

	"github.com/juju/juju/caas"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
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
		Cloud:          params.Cloud,
		Config:         cfg,
		ControllerUUID: bootstrapConfig.ControllerConfig.ControllerUUID(),
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
	Base           string
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

Using command arguments, it is possible to override cloud attributes region, 
endpoint, and base. By default, "amd64" is used for the architecture but this 
may also be changed.

Selecting an image for a specific base can be done via --base. --base can be 
specified using the OS name and the version of the OS, separated by @. For 
example, --base ubuntu@22.04.
`

func (c *imageMetadataCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "generate-image",
		Purpose: "generate simplestreams image metadata",
		Doc:     imageMetadataDoc,
	})
}

func (c *imageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Series, "s", "", "the charm series. DEPRECATED use --base")
	f.StringVar(&c.Base, "base", "", "the charm base")
	f.StringVar(&c.Arch, "a", arch.AMD64, "the image architecture")
	f.StringVar(&c.Dir, "d", "", "the destination directory in which to place the metadata files")
	f.StringVar(&c.ImageId, "i", "", "the image id")
	f.StringVar(&c.Region, "r", "", "the region")
	f.StringVar(&c.Endpoint, "u", "", "the cloud endpoint (for Openstack, this is the Identity Service endpoint)")
	f.StringVar(&c.Stream, "stream", imagemetadata.ReleasedStream, "the image stream")
	f.StringVar(&c.VirtType, "virt-type", "", "the image virtualisation type")
	f.StringVar(&c.Storage, "storage", "", "the type of root storage")
}

func (c *imageMetadataCommand) Init(args []string) error {
	if c.Series != "" && c.Base != "" {
		return errors.Errorf("cannot specify both base and series (series is deprecated)")
	}
	return nil
}

// setParams sets parameters based on the environment configuration
// for those which have not been explicitly specified.
func (c *imageMetadataCommand) setParams(context *cmd.Context, base corebase.Base) (corebase.Base, error) {
	c.privateStorage = "<private storage name>"

	controllerName, err := c.ControllerName()
	err = errors.Cause(err)
	if err != nil && err != modelcmd.ErrNoControllersDefined && !modelcmd.IsNoCurrentController(err) {
		return base, errors.Trace(err)
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
						return base, err
					}
				} else {
					return base, errors.Errorf("model %q cannot provide region and endpoint", environ.Config().Name())
				}
				// If only one of region or endpoint is provided, that is a problem.
				if cloudSpec.Region != cloudSpec.Endpoint && (cloudSpec.Region == "" || cloudSpec.Endpoint == "") {
					return base, errors.Errorf("cannot generate metadata without a complete cloud configuration")
				}
				if c.Region == "" {
					c.Region = cloudSpec.Region
				}
				if c.Endpoint == "" {
					c.Endpoint = cloudSpec.Endpoint
				}
			}

			// If we don't have a base set, then look up the one from the
			// environment configuration.
			if c.Base == "" {
				cfg := environ.Config()

				if b := config.PreferredBase(cfg); !b.Empty() {
					base = b
				}
			}
		} else {
			logger.Warningf("bootstrap parameters could not be opened: %v", err)
		}
	}
	if environ == nil {
		logger.Infof("no model found, creating image metadata using user supplied data")
	}
	if c.ImageId == "" {
		return base, errors.Errorf("image id must be specified")
	}
	if c.Region == "" {
		return base, errors.Errorf("image region must be specified")
	}
	if c.Endpoint == "" {
		return base, errors.Errorf("cloud endpoint URL must be specified")
	}
	if c.Dir == "" {
		logger.Infof("no destination directory specified, using current directory")
		var err error
		if c.Dir, err = os.Getwd(); err != nil {
			return base, err
		}
	}
	return base, nil
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

func (c *imageMetadataCommand) Run(ctx *cmd.Context) error {
	var (
		base corebase.Base
		err  error
	)
	// Note: we validated that both series and base cannot be specified in
	// Init(), so it's safe to assume that only one of them is set here.
	if c.Series != "" {
		ctx.Warningf("series flag is deprecated, use --base instead")
		if base, err = corebase.GetBaseFromSeries(c.Series); err != nil {
			return errors.Annotatef(err, "attempting to convert %q to a base", c.Series)
		}
		c.Base = base.String()
		c.Series = ""
	}
	if c.Base != "" {
		if base, err = corebase.ParseBaseFromString(c.Base); err != nil {
			return errors.Trace(err)
		}
	}
	if base.Empty() {
		base = version.DefaultSupportedLTSBase()
	}

	if base, err = c.setParams(ctx, base); err != nil {
		return err
	}
	out := ctx.Stdout
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
	err = imagemetadata.MergeAndWriteMetadata(fetcher, base, []*imagemetadata.ImageMetadata{im}, &cloudSpec, targetStorage)
	if err != nil {
		return errors.Errorf("image metadata files could not be created: %v", err)
	}
	dir := ctx.AbsPath(c.Dir)
	dest := filepath.Join(dir, storage.BaseImagesPath, "streams", "v1")
	fmt.Fprintf(out, helpDoc, dest, dir, dir)
	return nil
}
