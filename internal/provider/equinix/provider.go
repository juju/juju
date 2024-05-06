// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"context"
	stdcontext "context"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/schema"
	"github.com/packethost/packngo"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/version"
)

var _ environs.CloudEnvironProvider = (*environProvider)(nil)

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
func (p environProvider) Ping(ctx envcontext.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is part of the EnvironProvider interface.
func (p environProvider) PrepareConfig(ctx context.Context, args environs.PrepareConfigParams) (*config.Config, error) {
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

// Open opens the environment and returns it. The configuration must
// have passed through PrepareConfig at some point in its lifecycle.
//
// Open should not perform any expensive operations, such as querying
// the cloud API, as it will be called frequently.
//
// Open is part of the CloudEnvironProvider interface.
func (p environProvider) Open(ctx stdcontext.Context, args environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q", args.Config.Name())

	e := new(environ)
	e.name = args.Config.Name()

	if err := e.SetCloudSpec(args.Cloud); err != nil {
		return nil, errors.Trace(err)
	}

	if err := e.SetConfig(ctx, args.Config); err != nil {
		return nil, errors.Trace(err)
	}

	namespace, err := instance.NewNamespace(e.ecfg.config.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	e.namespace = namespace
	return e, nil
}

func (environProvider) Validate(_ context.Context, cfg, old *config.Config) (valid *config.Config, err error) {
	newEcfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid Equnix provider config: %v", err)
	}
	return newEcfg.config.Apply(newEcfg.attrs)
}

func (p environProvider) newConfig(ctx context.Context, cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(ctx, cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func (e *environ) SetCloudSpec(spec environscloudspec.CloudSpec) error {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.cloud = spec
	e.equinixClient = equinixClient(e.cloud)
	return nil
}

var equinixClient = func(spec environscloudspec.CloudSpec) *packngo.Client {
	credentialAttrs := spec.Credential.Attributes()

	apiToken := credentialAttrs["api-token"]
	httpClient := http.DefaultClient

	c := packngo.NewClientWithAuth("juju", apiToken, httpClient)
	userAgent := fmt.Sprintf("Juju/%s %s", version.Current.String(), c.UserAgent)
	c.UserAgent = strings.TrimSpace(userAgent)

	return c
}

func (environProvider) Version() int {
	return 1
}
