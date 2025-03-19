// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	stdcontext "context"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"golang.org/x/net/context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.provider.vmware")

const (
	// provider version 1 organises VMs into folders.
	providerVersion1 = 1

	currentProviderVersion = providerVersion1
)

type environProvider struct {
	environProviderCredentials
	dial DialFunc
}

var _ config.ConfigSchemaSource = (*environProvider)(nil)

// EnvironProviderConfig contains configuration for the EnvironProvider.
type EnvironProviderConfig struct {
	// Dial is a function used for dialing connections to vCenter/ESXi.
	Dial DialFunc
}

// NewEnvironProvider returns a new environs.EnvironProvider that will
// dial vSphere connections with the given dial function.
func NewEnvironProvider(config EnvironProviderConfig) environs.CloudEnvironProvider {
	return &environProvider{
		dial: config.Dial,
	}
}

// Version implements environs.EnvironProvider.
func (p *environProvider) Version() int {
	return currentProviderVersion
}

// Open implements environs.EnvironProvider.
func (p *environProvider) Open(ctx stdcontext.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(ctx, p, invalidator, args.Cloud, args.Config)
	return env, errors.Trace(err)
}

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
	Order:    []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: {
			Singular: "the vCenter address or URL",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
		cloud.AuthTypesKey: {
			// don't need a prompt, since there's only one choice.
			Type: []jsonschema.Type{jsonschema.ArrayType},
			Enum: []interface{}{[]string{string(cloud.UserPassAuthType)}},
		},
		cloud.RegionsKey: {
			Type:     []jsonschema.Type{jsonschema.ObjectType},
			Singular: "datacenter",
			Plural:   "datacenters",
			AdditionalProperties: &jsonschema.Schema{
				Type:          []jsonschema.Type{jsonschema.ObjectType},
				MaxProperties: jsonschema.Int(0),
			},
		},
	},
}

// CloudSchema returns the schema for adding new clouds of this type.
func (p *environProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

const failedLoginMsg = "ServerFaultCode: Cannot complete login due to an incorrect user name or password."

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p *environProvider) Ping(ctx stdcontext.Context, endpoint string) error {
	// try to be smart and not punish people for adding or forgetting http
	u, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("Invalid endpoint format, please give a full url or IP/hostname.")
	}
	switch u.Scheme {
	case "http", "https":
		// good!
	case "":
		u, err = url.Parse("https://" + endpoint + "/sdk")
		if err != nil {
			return errors.New("Invalid endpoint format, please give a full url or IP/hostname.")
		}
	default:
		return errors.New("Invalid endpoint format, please use an http or https URL.")
	}

	// Set a user, to force the dial function to perform a login. The login
	// should fail, since there's no password set.
	u.User = url.User("juju")

	client, err := p.dial(ctx, u, "")
	if err != nil {
		if err.Error() == failedLoginMsg {
			// This is our expected error for trying to log into
			// vSphere without any creds, so return nil.
			return nil
		}
		logger.Errorf(ctx, "Unexpected error dialing vSphere connection: %v", err)
		return errors.Errorf("No vCenter/ESXi available at %s", endpoint)
	}
	defer func() { _ = client.Close(ctx) }()

	// We shouldn't get here, since we haven't set a password, but it is
	// theoretically possible to have user="juju", password="".
	return nil
}

// ValidateCloud is specified in the EnvironProvider interface.
func (*environProvider) ValidateCloud(ctx context.Context, spec environscloudspec.CloudSpec) error {
	return errors.Annotate(validateCloudSpec(spec), "validating cloud spec")
}

// Validate implements environs.EnvironProvider.
func (*environProvider) Validate(ctx context.Context, cfg, old *config.Config) (valid *config.Config, err error) {
	if old == nil {
		ecfg, err := newValidConfig(ctx, cfg)
		if err != nil {
			return nil, errors.Annotate(err, "invalid config")
		}
		return ecfg.Config, nil
	}

	ecfg, err := newValidConfig(ctx, old)
	if err != nil {
		return nil, errors.Annotate(err, "invalid base config")
	}

	if err := ecfg.update(ctx, cfg); err != nil {
		return nil, errors.Annotate(err, "invalid config change")
	}

	return ecfg.Config, nil
}

func validateCloudSpec(spec environscloudspec.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	// TODO(axw) add validation of endpoint/region.
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.UserPassAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
