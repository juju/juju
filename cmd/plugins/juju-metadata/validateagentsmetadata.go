// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/output"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/version"
)

func newValidateToolsMetadataCommand() cmd.Command {
	return modelcmd.WrapController(&validateAgentsMetadataCommand{})
}

// validateAgentsMetadataCommand
type validateAgentsMetadataCommand struct {
	modelcmd.ControllerCommandBase

	out          cmd.Output
	providerType string
	metadataDir  string
	stream       string
	ostype       string
	region       string
	endpoint     string
	exactVersion string
	partVersion  string
	major        int
	minor        int
}

var validateAgentsMetadataDoc = `
validate-agent-binaries loads simplestreams metadata and validates the contents by
looking for agent binaries belonging to the specified os type and architecture, 
for the specified cloud. If version is specified, only agent binaries matching
that exact version will be considered. It is also possible to just specify the
major (and optionally minor) version numbers to search for.

The cloud specification comes from the current Juju model, as specified in
the usual way from either the -m option, or JUJU_MODEL. Release, Region, and
Endpoint are the key attributes.

It is possible to specify a local directory containing agent metadata, 
in which case cloud attributes like provider type, region etc are optional.

The key model attributes may be overridden using command arguments, so
that the validation may be performed on arbitrary metadata.

Examples:

 - validate using the current model settings but with os type windows
  
  juju metadata validate-agent-binaries -t windows

 - validate using the current model settings but with Juju version 1.11.4
  
  juju metadata validate-agent-binaries -j 1.11.4

 - validate using the current model settings but with Juju major version 2
  
  juju metadata validate-agent-binaries -m 2

 - validate using the current model settings but with Juju major.minor version 2.1
 
  juju metadata validate-agent-binaries -m 2.1

 - validate using the current model settings and list all agent binaries found 
   for any os type
 
  juju metadata validate-agent-binaries --os-type=

 - validate with os type windows and using metadata from local directory
 
  juju metadata validate-agent-binaries -t windows -d <some directory>

 - validate for the proposed stream

  juju metadata validate-agent-binaries --stream proposed

A key use case is to validate newly generated metadata prior to deployment to
production. In this case, the metadata is placed in a local directory, a cloud
provider type is specified (ec2, openstack etc), and the validation is performed
for each supported os type, version, and architecture.

Example bash snippet:

#!/bin/bash

juju metadata validate-agent-binaries -p ec2 -r us-east-1 -t ubuntu --juju-version 1.12.0 -d <some directory>
RETVAL=$?
[ $RETVAL -eq 0 ] && echo Success || echo "Failure"
`

func (c *validateAgentsMetadataCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "validate-agent-binaries",
		Purpose: "check that compressed tar archives (.tgz) for the Juju agent binaries are available",
		Doc:     validateAgentsMetadataDoc,
		SeeAlso: []string{
			"generate-images",
			"sign",
		},
	})
}

func (c *validateAgentsMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
	f.StringVar(&c.providerType, "p", "", "the provider type eg ec2, openstack")
	f.StringVar(&c.metadataDir, "d", "", "directory where metadata files are found")
	f.StringVar(&c.ostype, "t", "", "the os type for which to validate")
	f.StringVar(&c.ostype, "os-type", "", "")
	f.StringVar(&c.region, "r", "", "the region for which to validate (overrides model config region)")
	f.StringVar(&c.endpoint, "u", "", "the cloud endpoint URL for which to validate (overrides model config endpoint)")
	f.StringVar(&c.exactVersion, "j", "current", "the Juju version (use 'current' for current version)")
	f.StringVar(&c.exactVersion, "juju-version", "", "")
	f.StringVar(&c.partVersion, "majorminor-version", "", "")
	f.StringVar(&c.stream, "stream", tools.ReleasedStream, "simplestreams stream for which to generate the metadata")
}

func (c *validateAgentsMetadataCommand) Init(args []string) error {
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

func (c *validateAgentsMetadataCommand) Run(context *cmd.Context) error {
	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())

	var params *simplestreams.MetadataLookupParams
	if c.providerType == "" {
		context.Infof("no provider type specified, using bootstrapped cloud")
		controllerName, err := c.ControllerName()
		if err != nil {
			return errors.Trace(err)
		}
		environ, err := prepare(context, controllerName, c.ClientStore())
		if err == nil {
			mdLookup, ok := environ.(simplestreams.AgentMetadataValidator)
			if !ok {
				return errors.Errorf("%s provider does not support agent metadata validation", environ.Config().Type())
			}
			params, err = mdLookup.AgentMetadataLookupParams(c.region)
			if err != nil {
				return err
			}
			params.Sources, err = tools.GetMetadataSources(environ, ss)
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
		mdLookup, ok := prov.(simplestreams.AgentMetadataValidator)
		if !ok {
			return errors.Errorf("%s provider does not support metadata validation for agents", c.providerType)
		}
		params, err = mdLookup.AgentMetadataLookupParams(c.region)
		if err != nil {
			return err
		}
	}

	if len(params.Architectures) == 0 {
		params.Architectures = arch.AllSupportedArches
	}

	if c.ostype != "" {
		params.Release = c.ostype
	}
	if c.region != "" {
		params.Region = c.region
	}
	if c.endpoint != "" {
		params.Endpoint = c.endpoint
	}

	if c.metadataDir != "" {
		if _, err := c.Filesystem().Stat(c.metadataDir); err != nil {
			return err
		}
		toolsURL, err := tools.ToolsURL(c.metadataDir)
		if err != nil {
			return err
		}
		params.Sources = makeDataSources(ss, toolsURL)
	}
	params.Stream = c.stream

	versions, resolveInfo, err := tools.ValidateToolsMetadata(context, ss, &tools.ToolsMetadataLookupParams{
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
			"Matching Agent Binary Versions": versions,
			"Resolve Metadata":               *resolveInfo,
		}
		_ = c.out.Write(context, metadata)
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
