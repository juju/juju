// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"fmt"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/jsonschema"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	internallogger "github.com/juju/juju/internal/logger"
)

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey, cloud.AuthTypesKey},
	// Order doesn't matter since there's only one thing to ask about.  Add
	// order if this changes.
	Properties: map[string]*jsonschema.Schema{
		cloud.AuthTypesKey: {
			// don't need a prompt, since there's only one choice.
			Type: []jsonschema.Type{jsonschema.ArrayType},
			Enum: []interface{}{[]string{string(cloud.OAuth1AuthType)}},
		},
		cloud.EndpointKey: {
			Singular: "the API endpoint url",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
	},
}

// Logger for the MAAS provider.
var logger = internallogger.GetLogger("juju.provider.maas")

type EnvironProvider struct {
	environProviderCredentials

	// GetCapabilities is a function that connects to MAAS to return its set of
	// capabilities.
	GetCapabilities Capabilities
}

var _ environs.EnvironProvider = (*EnvironProvider)(nil)

var providerInstance EnvironProvider

// Version is part of the EnvironProvider interface.
func (EnvironProvider) Version() int {
	return 0
}

func (EnvironProvider) Open(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
	logger.Debugf(ctx, "opening model %q.", args.Config.Name())
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := NewEnviron(ctx, args.Cloud, args.Config, invalidator, nil)
	if err != nil {
		return nil, errors.Annotate(err, "creating MAAS environ")
	}
	return env, nil
}

// CloudSchema returns the schema for adding new clouds of this type.
func (p EnvironProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p EnvironProvider) Ping(ctx context.Context, endpoint string) error {
	var err error
	base, version, includesVersion := gomaasapi.SplitVersionedURL(endpoint)
	if includesVersion {
		err = p.checkMaas(ctx, base, version)
		if err == nil {
			return nil
		}
	} else {
		// No version info in the endpoint - try both in preference order.
		err = p.checkMaas(ctx, endpoint, apiVersion2)
		if err == nil {
			return nil
		}
	}
	return errors.Annotatef(err, "No MAAS server running at %s", endpoint)
}

func (p EnvironProvider) checkMaas(ctx context.Context, endpoint, ver string) error {
	c, err := gomaasapi.NewAnonymousClient(endpoint, ver)
	if err != nil {
		logger.Debugf(ctx, "Can't create maas API %s client for %q: %v", ver, endpoint, err)
		return errors.Trace(err)
	}
	maas := gomaasapi.NewMAAS(*c)
	_, err = p.GetCapabilities(ctx, maas, endpoint)
	return errors.Trace(err)
}

// ValidateCloud is specified in the EnvironProvider interface.
func (EnvironProvider) ValidateCloud(ctx context.Context, spec environscloudspec.CloudSpec) error {
	return errors.Annotate(validateCloudSpec(spec), "validating cloud spec")
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p EnvironProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

func validateCloudSpec(spec environscloudspec.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if _, err := parseCloudEndpoint(spec.Endpoint); err != nil {
		return errors.Annotate(err, "validating endpoint")
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.OAuth1AuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	if _, err := parseOAuthToken(*spec.Credential); err != nil {
		return errors.Annotate(err, "validating MAAS OAuth token")
	}
	return nil
}

func parseCloudEndpoint(endpoint string) (server string, _ error) {
	// For MAAS, the cloud endpoint may be either a full URL
	// for the MAAS server, or just the IP/host.
	if endpoint == "" {
		return "", errors.New("MAAS server not specified")
	}
	server = endpoint
	if url, err := url.Parse(server); err != nil || url.Scheme == "" {
		server = fmt.Sprintf("http://%s/MAAS", endpoint)
		if _, err := url.Parse(server); err != nil {
			return "", errors.NotValidf("endpoint %q", endpoint)
		}
	}
	return server, nil
}
