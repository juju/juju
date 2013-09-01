// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/version"
)

// ValidateToolsMetadataCommand
type ValidateToolsMetadataCommand struct {
	cmd.EnvCommandBase
	providerType string
	metadataDir  string
	series       string
	region       string
	endpoint     string
	version      string
}

var validateToolsMetadataDoc = `
validate-tools loads simplestreams metadata and validates the contents by looking for tools
belonging to the specified series, version, and architecture, for the specified cloud.

The cloud specificaton comes from the current Juju environment, as specified in the usual way
from either ~/.juju/environments.yaml, the -e option, or JUJU_ENV. Series, Region, and Endpoint
are the key attributes.

The key environment attributes may be overridden using command arguments, so that the validation
may be peformed on arbitary metadata.

Examples:

- validate using the current environment settings but with series raring
 juju metadata validate-tools -s raring

- validate using the current environment settings but with Juju version 1.11.4
 juju metadata validate-tools -j 1.11.4

- validate using the current environment settings and list all tools found for any series
 juju metadata validate-tools --series=

- validate using the current environment settings but with series raring and using metadata from local directory
 juju metadata validate-images -s raring -d <some directory>

A key use case is to validate newly generated metadata prior to deployment to production.
In this case, the metadata is placed in a local directory, a cloud provider type is specified (ec2, openstack etc),
and the validation is performed for each supported series, version, and arcgitecture.

Example bash snippet:

#!/bin/bash

juju metadata validate-tools -p ec2 -r us-east-1 -s precise --juju-version 1.12.0 -d <some directory>
RETVAL=$?
[ $RETVAL -eq 0 ] && echo Success
[ $RETVAL -ne 0 ] && echo Failure
`

func (c *ValidateToolsMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "validate-tools",
		Purpose: "validate tools metadata and ensure tools tarball(s) exist for Juju version(s)",
		Doc:     validateToolsMetadataDoc,
	}
}

func (c *ValidateToolsMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.providerType, "p", "", "the provider type eg ec2, openstack")
	f.StringVar(&c.metadataDir, "d", "", "directory where metadata files are found")
	f.StringVar(&c.series, "s", "", "the series for which to validate (overrides env config series)")
	f.StringVar(&c.series, "series", "", "")
	f.StringVar(&c.region, "r", "", "the region for which to validate (overrides env config region)")
	f.StringVar(&c.endpoint, "u", "", "the cloud endpoint URL for which to validate (overrides env config endpoint)")
	f.StringVar(&c.version, "j", "current", "the Juju version (use 'current' for current version)")
	f.StringVar(&c.version, "juju-version", "", "")
}

func (c *ValidateToolsMetadataCommand) Init(args []string) error {
	if c.providerType != "" {
		if c.region == "" {
			return fmt.Errorf("region required if provider type is specified")
		}
		if c.metadataDir == "" {
			return fmt.Errorf("metadata directory required if provider type is specified")
		}
	}
	if c.version == "current" {
		c.version = version.CurrentNumber().String()
	}
	return c.EnvCommandBase.Init(args)
}

func (c *ValidateToolsMetadataCommand) Run(context *cmd.Context) error {
	var params *simplestreams.MetadataLookupParams

	if c.providerType == "" {
		environ, err := environs.NewFromName(c.EnvName)
		if err != nil {
			return err
		}
		mdLookup, ok := environ.(simplestreams.MetadataValidator)
		if !ok {
			return fmt.Errorf("%s provider does not support tools metadata validation", environ.Config().Type())
		}
		params, err = mdLookup.MetadataLookupParams(c.region)
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
			return fmt.Errorf("%s provider does not support tools metadata validation", c.providerType)
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
	// If the metadata files are to be loaded from a directory, we need to register
	// a file http transport.
	if c.metadataDir != "" {
		if _, err := os.Stat(c.metadataDir); err != nil {
			return err
		}

		params.BaseURLs = []string{"file://" + c.metadataDir}
		t := &http.Transport{}
		t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
		c := &http.Client{Transport: t}
		simplestreams.SetHttpClient(c)
	}

	versions, err := tools.ValidateToolsMetadata(&tools.ToolsMetadataLookupParams{
		MetadataLookupParams: *params,
		Version:              c.version,
	})
	if err != nil {
		return err
	}

	if len(versions) > 0 {
		fmt.Fprintf(context.Stdout, "matching tools versions:\n%s\n", strings.Join(versions, "\n"))
	} else {
		return fmt.Errorf("no matching tools using URLs:\n%s", strings.Join(params.BaseURLs, "\n"))
	}
	return nil
}
