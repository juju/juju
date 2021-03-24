// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/schema"
	"github.com/lxc/lxd/shared/logger"
	"github.com/packethost/packngo"
)

type environProvider struct {
	environProviderCredentials
}

func (p environProvider) ConfigSchema() schema.Fields {
	return configFields
}

func (p environProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p environProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is part of the EnvironProvider interface.
func (p environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	return args.Config, nil
}

func validateCloudSpec(spec environscloudspec.CloudSpec) error {
	credentialAttrs := spec.Credential.Attributes()

	projectID := credentialAttrs["project-id"]
	apiToken := credentialAttrs["api-token"]

	if apiToken == "" {
		return fmt.Errorf("api-token not present")
	}

	if projectID == "" {
		return fmt.Errorf("project-id not present")
	}

	return nil
}

// Open is specified in the EnvironProvider interface.
func (p environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q", args.Config.Name())

	e := new(environ)
	e.name = args.Config.Name()

	if err := e.SetCloudSpec(args.Cloud); err != nil {
		return nil, err
	}

	if err := e.SetConfig(args.Config); err != nil {
		return nil, errors.Trace(err)
	}
	return e, nil
}

func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	newEcfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid Equnix provider config: %v", err)
	}
	return newEcfg.config.Apply(newEcfg.attrs)
}

func (p environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func (e *environ) SetCloudSpec(spec environscloudspec.CloudSpec) error {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()

	e.cloud = spec

	// var err error
	e.equnixClient = equnixClient(e.cloud)

	return nil
}

func equnixClient(spec environscloudspec.CloudSpec) *packngo.Client {
	credentialAttrs := spec.Credential.Attributes()

	apiToken := credentialAttrs["api-token"]
	httpClient := http.DefaultClient

	c := packngo.NewClientWithAuth("juju", apiToken, httpClient)

	return c
}

func (environProvider) Version() int {
	return 0
}
