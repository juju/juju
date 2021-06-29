// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	stdcontext "context"
	"fmt"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
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
var logger = loggo.GetLogger("juju.provider.maas")

type MaasEnvironProvider struct {
	environProviderCredentials

	// GetCapabilities is a function that connects to MAAS to return its set of
	// capabilities.
	GetCapabilities MaasCapabilities
}

var _ environs.EnvironProvider = (*MaasEnvironProvider)(nil)

var providerInstance MaasEnvironProvider

// Version is part of the EnvironProvider interface.
func (MaasEnvironProvider) Version() int {
	return 0
}

func (MaasEnvironProvider) Open(_ stdcontext.Context, args environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q.", args.Config.Name())
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := NewEnviron(args.Cloud, args.Config, nil)
	if err != nil {
		return nil, err
	}
	return env, nil
}

// CloudSchema returns the schema for adding new clouds of this type.
func (p MaasEnvironProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p MaasEnvironProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	base, version, includesVersion := gomaasapi.SplitVersionedURL(endpoint)
	if includesVersion {
		err := p.checkMaas(base, version)
		if err == nil {
			return nil
		}
	} else {
		// No version info in the endpoint - try both in preference order.
		err := p.checkMaas(endpoint, apiVersion2)
		if err == nil {
			return nil
		}
		err = p.checkMaas(endpoint, apiVersion1)
		if err == nil {
			return nil
		}
	}
	return errors.Errorf("No MAAS server running at %s", endpoint)
}

func (p MaasEnvironProvider) checkMaas(endpoint, ver string) error {
	c, err := gomaasapi.NewAnonymousClient(endpoint, ver)
	if err != nil {
		logger.Debugf("Can't create maas API %s client for %q: %v", ver, endpoint, err)
		return errors.Trace(err)
	}
	maas := gomaasapi.NewMAAS(*c)
	_, err = p.GetCapabilities(maas, endpoint)
	return errors.Trace(err)
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p MaasEnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	var attrs map[string]interface{}
	if _, ok := args.Config.StorageDefaultBlockSource(); !ok {
		attrs = map[string]interface{}{
			config.StorageDefaultBlockSourceKey: maasStorageProviderType,
		}
	}
	if len(attrs) == 0 {
		return args.Config, nil
	}
	return args.Config.Apply(attrs)
}

func verifyCredentials(env *maasEnviron, ctx context.ProviderCallContext) error {
	// Verify we can connect to the server and authenticate.
	if env.usingMAAS2() {
		// The maas2 controller verifies credentials at creation time.
		return nil
	}
	_, err := env.getMAASClient().GetSubObject("maas").CallGet("get_config", nil)
	if denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx); denied {
		logger.Debugf("authentication failed: %v", err)
		return errors.New(`authentication failed.

Please ensure the credentials are correct.`)
	}
	return nil
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p MaasEnvironProvider) DetectRegions() ([]cloud.Region, error) {
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
