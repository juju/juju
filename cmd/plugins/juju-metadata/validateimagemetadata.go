// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
)

func newValidateImageMetadataCommand() cmd.Command {
	return modelcmd.WrapController(&validateImageMetadataCommand{})
}

// validateImageMetadataCommand
type validateImageMetadataCommand struct {
	modelcmd.ControllerCommandBase

	out          cmd.Output
	providerType string
	metadataDir  string
	series       string
	base         string
	region       string
	endpoint     string
	stream       string
}

var validateImagesMetadataDoc = `
validate-images loads simplestreams metadata and validates the contents by
looking for images belonging to the specified cloud.

The cloud specification comes from the current Juju model, as specified in
the usual way from either the -m option, or JUJU_MODEL. Release, Region, and
Endpoint are the key attributes.

The key model attributes may be overridden using command arguments, so
that the validation may be performed on arbitrary metadata.

Examples:

 - validate using the current model settings but with base ubuntu@22.04

  juju metadata validate-images --base ubuntu@22.04

 - validate using the current model settings but with base ubuntu@22.04 and
 using metadata from local directory (the directory is expected to have an
 "images" subdirectory containing the metadata, and corresponds to the parameter
 passed to the image metadata generatation command).

  juju metadata validate-images --base ubuntu@22.04 -d <some directory>

A key use case is to validate newly generated metadata prior to deployment to
production. In this case, the metadata is placed in a local directory, a cloud
provider type is specified (ec2, openstack etc), and the validation is performed
for each supported region and base.

Validating an image for a specific base can be done via --base. --base can be 
specified using the OS name and the version of the OS, separated by @. For 
example, --base ubuntu@22.04.

Example bash snippet:

#!/bin/bash

juju metadata validate-images -p ec2 -r us-east-1 -d <some directory>
RETVAL=$?
[ $RETVAL -eq 0 ] && echo Success
[ $RETVAL -ne 0 ] && echo Failure
`

func (c *validateImageMetadataCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "validate-images",
		Purpose: "validate image metadata and ensure image(s) exist for a model",
		Doc:     validateImagesMetadataDoc,
	})
}

func (c *validateImageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
	f.StringVar(&c.providerType, "p", "", "the provider type eg ec2, openstack")
	f.StringVar(&c.metadataDir, "d", "", "directory where metadata files are found")
	f.StringVar(&c.series, "s", "", "the series for which to validate (overrides model config series). DEPRECATED use --base")
	f.StringVar(&c.base, "base", "", "the base for which to validate (overrides model config base)")
	f.StringVar(&c.region, "r", "", "the region for which to validate (overrides model config region)")
	f.StringVar(&c.endpoint, "u", "", "the cloud endpoint URL for which to validate (overrides model config endpoint)")
	f.StringVar(&c.stream, "stream", "", "the images stream (defaults to released)")
}

func (c *validateImageMetadataCommand) Init(args []string) error {
	if c.base != "" && c.series != "" {
		return errors.New("--series and --base cannot be specified together")
	}
	if c.providerType != "" {
		if c.base == "" && c.series == "" {
			return errors.Errorf("base required if provider type is specified")
		}
		if c.region == "" {
			return errors.Errorf("region required if provider type is specified")
		}
		if c.metadataDir == "" {
			return errors.Errorf("metadata directory required if provider type is specified")
		}
	}
	return cmd.CheckEmpty(args)
}

var _ environs.ConfigGetter = (*overrideEnvStream)(nil)

// overrideEnvStream implements environs.ConfigGetter and
// ensures that the environs.Config returned by Config()
// has the specified stream.
type overrideEnvStream struct {
	environs.Environ
	stream string
}

func (oes *overrideEnvStream) Config() *config.Config {
	cfg := oes.Environ.Config()
	// If no stream specified, just use default from environ.
	if oes.stream == "" {
		return cfg
	}
	newCfg, err := cfg.Apply(map[string]interface{}{"image-stream": oes.stream})
	if err != nil {
		// This should never happen.
		panic(errors.Errorf("unexpected error making override config: %v", err))
	}
	return newCfg
}

func (c *validateImageMetadataCommand) Run(ctx *cmd.Context) error {
	var (
		base corebase.Base
		err  error
	)
	// Note: we validated that both series and base cannot be specified in
	// Init(), so it's safe to assume that only one of them is set here.
	if c.series != "" {
		ctx.Warningf("series flag is deprecated, use --base instead")
		if base, err = corebase.GetBaseFromSeries(c.series); err != nil {
			return errors.Annotatef(err, "attempting to convert %q to a base", c.series)
		}
		c.base = base.String()
		c.series = ""
	}
	if c.base != "" {
		if base, err = corebase.ParseBaseFromString(c.base); err != nil {
			return errors.Trace(err)
		}
	}

	params, err := c.createLookupParams(ctx, base)
	if err != nil {
		return err
	}

	fetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	images, resolveInfo, err := imagemetadata.ValidateImageMetadata(fetcher, params)
	if err != nil {
		if resolveInfo != nil {
			metadata := map[string]interface{}{
				"Resolve Metadata": *resolveInfo,
			}
			buff := &bytes.Buffer{}
			if yamlErr := cmd.FormatYaml(buff, metadata); yamlErr == nil {
				err = errors.Errorf("%v\n%v", err, buff.String())
			}
		}
		return err
	}
	if len(images) > 0 {
		metadata := map[string]interface{}{
			"ImageIds":         images,
			"Region":           params.Region,
			"Resolve Metadata": *resolveInfo,
		}
		_ = c.out.Write(ctx, metadata)
	} else {
		var sources []string
		for _, s := range params.Sources {
			url, err := s.URL("")
			if err == nil {
				sources = append(sources, fmt.Sprintf("- %s (%s)", s.Description(), url))
			}
		}
		return errors.Errorf(
			"no matching image ids for region %s using sources:\n%s",
			params.Region, strings.Join(sources, "\n"))
	}
	return nil
}

func (c *validateImageMetadataCommand) createLookupParams(context *cmd.Context, base corebase.Base) (*simplestreams.MetadataLookupParams, error) {
	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())

	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}

	params := &simplestreams.MetadataLookupParams{Stream: c.stream}
	if c.providerType == "" {
		environ, err := prepare(context, controllerName, c.ClientStore())
		if err != nil {
			return nil, err
		}
		mdLookup, ok := environ.(simplestreams.ImageMetadataValidator)
		if !ok {
			return nil, errors.Errorf("%s provider does not support image metadata validation", environ.Config().Type())
		}
		params, err = mdLookup.ImageMetadataLookupParams(c.region)
		if err != nil {
			return nil, err
		}
		oes := &overrideEnvStream{environ, c.stream}
		params.Sources, err = environs.ImageMetadataSources(oes, ss)
		if err != nil {
			return nil, err
		}
	} else {
		prov, err := environs.Provider(c.providerType)
		if err != nil {
			return nil, err
		}
		mdLookup, ok := prov.(simplestreams.ImageMetadataValidator)
		if !ok {
			return nil, errors.Errorf("%s provider does not support image metadata validation", c.providerType)
		}
		params, err = mdLookup.ImageMetadataLookupParams(c.region)
		if err != nil {
			return nil, err
		}
	}

	if !base.Empty() {
		params.Release = base.Channel.Track
	}
	if c.region != "" {
		params.Region = c.region
	}
	if c.endpoint != "" {
		params.Endpoint = c.endpoint
	}
	if c.metadataDir != "" {
		dir := filepath.Join(c.metadataDir, "images")
		if _, err := c.Filesystem().Stat(dir); err != nil {
			return nil, err
		}
		params.Sources = imagesDataSources(ss, dir)
	}
	return params, nil
}

var imagesDataSources = func(ss *simplestreams.Simplestreams, urls ...string) []simplestreams.DataSource {
	dataSources := make([]simplestreams.DataSource, len(urls))
	publicKey, _ := simplestreams.UserPublicSigningKey()
	for i, url := range urls {
		dataSources[i] = ss.NewDataSource(
			simplestreams.Config{
				Description:          "local metadata directory",
				BaseURL:              "file://" + url,
				PublicSigningKey:     publicKey,
				HostnameVerification: true,
				Priority:             simplestreams.CUSTOM_CLOUD_DATA,
			},
		)
	}
	return dataSources
}
