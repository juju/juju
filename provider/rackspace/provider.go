// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"io"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type environProvider struct {
	environs.EnvironProvider
}

var providerInstance *environProvider

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p environProvider) Ping(in io.Reader, out io.Writer, authorizedKeys, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is part of the EnvironProvider interface.
func (p *environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	args.Cloud = transformCloudSpec(args.Cloud)
	return p.EnvironProvider.PrepareConfig(args)
}

// Open is part of the EnvironProvider interface.
func (p *environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	args.Cloud = transformCloudSpec(args.Cloud)
	return p.EnvironProvider.Open(args)
}

func transformCloudSpec(spec environs.CloudSpec) environs.CloudSpec {
	// Rackspace regions are expected to be uppercase, but Juju
	// stores and displays them in lowercase in the CLI. Ensure
	// they're uppercase when they get to the Rackspace API.
	spec.Region = strings.ToUpper(spec.Region)
	return spec
}
