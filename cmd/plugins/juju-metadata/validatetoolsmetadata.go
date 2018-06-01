// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/arch"
	"github.com/juju/version"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	jujuversion "github.com/juju/juju/version"
)

func newValidateToolsMetadataCommand() cmd.Command {
	return modelcmd.Wrap(&validateToolsMetadataCommand{})
}

// validateToolsMetadataCommand
type validateToolsMetadataCommand struct {
	imageMetadataCommandBase
	out          cmd.Output
	providerType string
	metadataDir  string
	stream       string
	series       string
	region       string
	endpoint     string
	exactVersion string
	partVersion  string
	major        int
	minor        int
}

var validateToolsMetadataDoc = `
validate-agents loads simplestreams metadata and validates the contents by
looking for agent binaries belonging to the specified series, architecture, for the
specified cloud. If version is specified, agent binaries matching the exact specified
version are found. It is also possible to just specify the major (and optionally
minor) version numbers to search for.

The cloud specification comes from the current Juju model, as specified in
the usual way from either the -m option, or JUJU_MODEL. Series, Region, and
Endpoint are the key attributes.

It is possible to specify a local directory containing agent metadata, 
in which case cloud attributes like provider type, region etc are optional.

The key model attributes may be overridden using command arguments, so
that the validation may be performed on arbitrary metadata.

Examples:

 - validate using the current model settings but with series raring
  
  juju metadata validate-agents -s raring

 - validate using the current model settings but with Juju version 1.11.4
  
  juju metadata validate-agents -j 1.11.4

 - validate using the current model settings but with Juju major version 2
  
  juju metadata validate-agents -m 2

 - validate using the current model settings but with Juju major.minor version 2.1
 
  juju metadata validate-agents -m 2.1

 - validate using the current model settings and list all agent binaries found for any series
 
  juju metadata validate-agents --series=

 - validate with series raring and using metadata from local directory
 
  juju metadata validate-agents -s raring -d <some directory>

 - validate for the proposed stream

  juju metadata validate-agents --stream proposed

A key use case is to validate newly generated metadata prior to deployment to
production. In this case, the metadata is placed in a local directory, a cloud
provider type is specified (ec2, openstack etc), and the validation is performed
for each supported series, version, and arcgitecture.

Example bash snippet:

#!/bin/bash

juju metadata validate-agents -p ec2 -r us-east-1 -s precise --juju-version 1.12.0 -d <some directory>
RETVAL=$?
[ $RETVAL -eq 0 ] && echo Success
[ $RETVAL -ne 0 ] && echo Failure
`

func (c *validateToolsMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "validate-agents",
		Purpose: "validate agent metadata and ensure agent binary tarball(s) exist for Juju version(s)",
		Doc:     validateToolsMetadataDoc,
		Aliases: []string{"validate-tools"},
	}
}

func (c *validateToolsMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
	f.StringVar(&c.providerType, "p", "", "the provider type eg ec2, openstack")
	f.StringVar(&c.metadataDir, "d", "", "directory where metadata files are found")
	f.StringVar(&c.series, "s", "", "the series for which to validate (overrides env config series)")
	f.StringVar(&c.series, "series", "", "")
	f.StringVar(&c.region, "r", "", "the region for which to validate (overrides env config region)")
	f.StringVar(&c.endpoint, "u", "", "the cloud endpoint URL for which to validate (overrides env config endpoint)")
	f.StringVar(&c.exactVersion, "j", "current", "the Juju version (use 'current' for current version)")
	f.StringVar(&c.exactVersion, "juju-version", "", "")
	f.StringVar(&c.partVersion, "majorminor-version", "", "")
	f.StringVar(&c.stream, "stream", tools.ReleasedStream, "simplestreams stream for which to generate the metadata")
}

func (c *validateToolsMetadataCommand) Init(args []string) error {
	if c.providerType != "" {
		if c.region == "" {
			return errors.Errorf("region required if provider type is specified")
		}
		if c.metadataDir == "" {
			return errors.Errorf("metadata directory required if provider type is specified")
		}
	}
	if c.exactVersion == "current" {
		c.exactVersion = jujuversion.Current.String()
	}
	if c.partVersion != "" {
		var err error
		if c.major, c.minor, err = version.ParseMajorMinor(c.partVersion); err != nil {
			return err
		}
	}
	return cmd.CheckEmpty(args)
}

func (c *validateToolsMetadataCommand) Run(context *cmd.Context) error {
	var params *simplestreams.MetadataLookupParams

	if c.providerType == "" {
		environ, err := c.prepare(context)
		if err == nil {
			mdLookup, ok := environ.(simplestreams.MetadataValidator)
			if !ok {
				return errors.Errorf("%s provider does not support agent metadata validation", environ.Config().Type())
			}
			params, err = mdLookup.MetadataLookupParams(c.region)
			if err != nil {
				return err
			}
			params.Sources, err = tools.GetMetadataSources(environ)
			if err != nil {
				return err
			}
		} else {
			if c.metadataDir == "" {
				return err
			}
			params = &simplestreams.MetadataLookupParams{}
		}
	} else {
		prov, err := environs.Provider(c.providerType)
		if err != nil {
			return err
		}
		mdLookup, ok := prov.(simplestreams.MetadataValidator)
		if !ok {
			return errors.Errorf("%s provider does not support metadata validation for agents", c.providerType)
		}
		params, err = mdLookup.MetadataLookupParams(c.region)
		if err != nil {
			return err
		}
	}

	if len(params.Architectures) == 0 {
		params.Architectures = arch.AllSupportedArches
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
		if _, err := os.Stat(c.metadataDir); err != nil {
			return err
		}
		toolsURL, err := tools.ToolsURL(c.metadataDir)
		if err != nil {
			return err
		}
		params.Sources = toolsDataSources(toolsURL)
	}
	params.Stream = c.stream

	versions, resolveInfo, err := tools.ValidateToolsMetadata(&tools.ToolsMetadataLookupParams{
		MetadataLookupParams: *params,
		Version:              c.exactVersion,
		Major:                c.major,
		Minor:                c.minor,
	})
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

	if len(versions) > 0 {
		metadata := map[string]interface{}{
			"Matching Tools Versions": versions,
			"Resolve Metadata":        *resolveInfo,
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
		return errors.Errorf("no matching agent binaries using sources:\n%s", strings.Join(sources, "\n"))
	}
	return nil
}
