// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
)

// ValidateImageMetadataCommand
type ValidateImageMetadataCommand struct {
	cmd.EnvCommandBase
	out          cmd.Output
	providerType string
	metadataDir  string
	series       string
	region       string
	endpoint     string
	stream       string
}

var validateImagesMetadataDoc = `
validate-images loads simplestreams metadata and validates the contents by
looking for images belonging to the specified cloud.

The cloud specification comes from the current Juju environment, as specified in
the usual way from either ~/.juju/environments.yaml, the -e option, or JUJU_ENV.
Series, Region, and Endpoint are the key attributes.

The key environment attributes may be overridden using command arguments, so
that the validation may be peformed on arbitary metadata.

Examples:

 - validate using the current environment settings but with series raring

  juju metadata validate-images -s raring

 - validate using the current environment settings but with series raring and
 using metadata from local directory (the directory is expected to have an
 "images" subdirectory containing the metadata, and corresponds to the parameter
 passed to the image metadata generatation command).

  juju metadata validate-images -s raring -d <some directory>

A key use case is to validate newly generated metadata prior to deployment to
production. In this case, the metadata is placed in a local directory, a cloud
provider type is specified (ec2, openstack etc), and the validation is performed
for each supported region and series.

Example bash snippet:

#!/bin/bash

juju metadata validate-images -p ec2 -r us-east-1 -s precise -d <some directory>
RETVAL=$?
[ $RETVAL -eq 0 ] && echo Success
[ $RETVAL -ne 0 ] && echo Failure
`

func (c *ValidateImageMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "validate-images",
		Purpose: "validate image metadata and ensure image(s) exist for an environment",
		Doc:     validateImagesMetadataDoc,
	}
}

func (c *ValidateImageMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.StringVar(&c.providerType, "p", "", "the provider type eg ec2, openstack")
	f.StringVar(&c.metadataDir, "d", "", "directory where metadata files are found")
	f.StringVar(&c.series, "s", "", "the series for which to validate (overrides env config series)")
	f.StringVar(&c.region, "r", "", "the region for which to validate (overrides env config region)")
	f.StringVar(&c.endpoint, "u", "", "the cloud endpoint URL for which to validate (overrides env config endpoint)")
	f.StringVar(&c.stream, "m", "", "the images stream (defaults to released)")
}

func (c *ValidateImageMetadataCommand) Init(args []string) error {
	if c.providerType != "" {
		if c.series == "" {
			return fmt.Errorf("series required if provider type is specified")
		}
		if c.region == "" {
			return fmt.Errorf("region required if provider type is specified")
		}
		if c.metadataDir == "" {
			return fmt.Errorf("metadata directory required if provider type is specified")
		}
	}
	return c.EnvCommandBase.Init(args)
}

var _ environs.ConfigGetter = (*overrideEnvStream)(nil)

// overrideEnvStream implements environs.ConfigGetter and
// ensures that the environs.Config returned by Config()
// has the specified stream.
type overrideEnvStream struct {
	env    environs.Environ
	stream string
}

func (oes *overrideEnvStream) Config() *config.Config {
	cfg := oes.env.Config()
	// If no stream specified, just use default from environ.
	if oes.stream == "" {
		return cfg
	}
	newCfg, err := cfg.Apply(map[string]interface{}{"image-stream": oes.stream})
	if err != nil {
		// This should never happen.
		panic(fmt.Errorf("unexpected error making override config: %v", err))
	}
	return newCfg
}

func (c *ValidateImageMetadataCommand) Run(context *cmd.Context) error {
	var params *simplestreams.MetadataLookupParams

	if c.providerType == "" {
		store, err := configstore.Default()
		if err != nil {
			return err
		}
		environ, err := environs.PrepareFromName(c.EnvName, context, store)
		if err != nil {
			return err
		}
		mdLookup, ok := environ.(simplestreams.MetadataValidator)
		if !ok {
			return fmt.Errorf("%s provider does not support image metadata validation", environ.Config().Type())
		}
		params, err = mdLookup.MetadataLookupParams(c.region)
		if err != nil {
			return err
		}
		oes := &overrideEnvStream{environ, c.stream}
		params.Sources, err = imagemetadata.GetMetadataSources(oes)
		if err != nil {
			return err
		}
	} else {
		prov, err := environs.Provider(c.providerType)
		if err != nil {
			return err
		}
		mdLookup, ok := prov.(simplestreams.MetadataValidator)
		if !ok {
			return fmt.Errorf("%s provider does not support image metadata validation", c.providerType)
		}
		params, err = mdLookup.MetadataLookupParams(c.region)
		if err != nil {
			return err
		}
	}

	if c.series != "" {
		params.Series = c.series
	}
	if c.region != "" {
		params.Region = c.region
	}
	if c.endpoint != "" {
		params.Endpoint = c.endpoint
	}
	if c.metadataDir != "" {
		dir := filepath.Join(c.metadataDir, "images")
		if _, err := os.Stat(dir); err != nil {
			return err
		}
		params.Sources = []simplestreams.DataSource{
			simplestreams.NewURLDataSource(
				"local metadata directory", "file://"+dir, simplestreams.VerifySSLHostnames),
		}
	}
	params.Stream = c.stream

	image_ids, resolveInfo, err := imagemetadata.ValidateImageMetadata(params)
	if err != nil {
		if resolveInfo != nil {
			metadata := map[string]interface{}{
				"Resolve Metadata": *resolveInfo,
			}
			if metadataYaml, yamlErr := cmd.FormatYaml(metadata); yamlErr == nil {
				err = fmt.Errorf("%v\n%v", err, string(metadataYaml))
			}
		}
		return err
	}
	if len(image_ids) > 0 {
		metadata := map[string]interface{}{
			"ImageIds":         image_ids,
			"Region":           params.Region,
			"Resolve Metadata": *resolveInfo,
		}
		c.out.Write(context, metadata)
	} else {
		var sources []string
		for _, s := range params.Sources {
			url, err := s.URL("")
			if err == nil {
				sources = append(sources, fmt.Sprintf("- %s (%s)", s.Description(), url))
			}
		}
		return fmt.Errorf(
			"no matching image ids for region %s using sources:\n%s",
			params.Region, strings.Join(sources, "\n"))
	}
	return nil
}
