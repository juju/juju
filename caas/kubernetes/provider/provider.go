// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type kubernetesEnvironProvider struct {
	environProviderCredentials
}

var _ environs.EnvironProvider = (*kubernetesEnvironProvider)(nil)

// Version is part of the EnvironProvider interface.
func (kubernetesEnvironProvider) Version() int {
	return 0
}

// Open is part of the ContainerEnvironProvider interface.
func (kubernetesEnvironProvider) Open(args environs.OpenParams) (caas.Broker, error) {
	logger.Debugf("opening model %q.", args.Config.Name())
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	broker, err := NewK8sProvider(args.Cloud, args.Config.Name())
	if err != nil {
		return nil, err
	}
	return broker, nil
}

// CloudSchema returns the schema for adding new clouds of this type.
func (p kubernetesEnvironProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p kubernetesEnvironProvider) Ping(endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p kubernetesEnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	return args.Config, nil
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p kubernetesEnvironProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

func (p kubernetesEnvironProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if _, err := url.Parse(spec.Endpoint); err != nil {
		return errors.NotValidf("endpoint %q", spec.Endpoint)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.UserPassAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
