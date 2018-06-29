// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/schema"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/lxd/lxdnames"
)

type environProvider struct {
	environs.ProviderCredentials
	serverFactory ServerFactory
	Clock         clock.Clock
}

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey, cloud.AuthTypesKey},
	// Order doesn't matter since there's only one thing to ask about.  Add
	// order if this changes.
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: {
			Singular: "the API endpoint url for the remote LXD cloud",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
		cloud.AuthTypesKey: {
			Singular:    "auth type",
			Plural:      "auth types",
			Type:        []jsonschema.Type{jsonschema.ArrayType},
			UniqueItems: jsonschema.Bool(true),
			Items: &jsonschema.ItemSpec{
				Schemas: []*jsonschema.Schema{{
					Type: []jsonschema.Type{jsonschema.StringType},
					Enum: []interface{}{
						string(cloud.CertificateAuthType),
						string(interactiveAuthType),
					},
				}},
			},
		},
	},
}

// NewProvider returns a new LXD EnvironProvider.
func NewProvider() environs.CloudEnvironProvider {
	factory := NewServerFactory()
	return &environProvider{
		ProviderCredentials: environProviderCredentials{
			certReadWriter: certificateReadWriter{},
			certGenerator:  certificateGenerator{},
			lookup:         netLookup{},
			serverFactory:  factory,
		},
		serverFactory: factory,
	}
}

// Version is part of the EnvironProvider interface.
func (*environProvider) Version() int {
	return 0
}

// Open implements environs.EnvironProvider.
func (p *environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	local, err := p.validateCloudSpec(args.Cloud)
	if err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(
		p,
		local,
		args.Cloud,
		args.Config,
		p.serverFactory,
	)
	return env, errors.Trace(err)
}

// CloudSchema returns the schema used to validate input for add-cloud.
func (p *environProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p *environProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	// if the endpoint is empty, then don't ping, as we can assume we're using
	// local lxd
	if endpoint == "" {
		return nil
	}

	// Connect to the remote server anonymously so we can just verify it exists
	// as we're not sure that the certificates are loaded in time for when the
	// ping occurs i.e. interactive add-cloud
	_, err := lxd.ConnectRemote(lxd.NewInsecureServerSpec(endpoint))
	if err != nil {
		return errors.Errorf("no lxd server running at %s", endpoint)
	}
	return nil
}

// PrepareConfig implements environs.EnvironProvider.
func (p *environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	_, err := p.validateCloudSpec(args.Cloud)
	if err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	// Set the default filesystem-storage source.
	attrs := make(map[string]interface{})
	if _, ok := args.Config.StorageDefaultFilesystemSource(); !ok {
		attrs[config.StorageDefaultFilesystemSourceKey] = lxdStorageProviderType
	}
	if len(attrs) == 0 {
		return args.Config, nil
	}
	cfg, err := args.Config.Apply(attrs)
	return cfg, errors.Trace(err)
}

// Validate implements environs.EnvironProvider.
func (*environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	if _, err := newValidConfig(cfg); err != nil {
		return nil, errors.Annotate(err, "invalid base config")
	}
	return cfg, nil
}

// DetectClouds implements environs.CloudDetector.
func (p *environProvider) DetectClouds() ([]cloud.Cloud, error) {
	return []cloud.Cloud{localhostCloud}, nil
}

// DetectCloud implements environs.CloudDetector.
func (p *environProvider) DetectCloud(name string) (cloud.Cloud, error) {
	// For now we just return a hard-coded "localhost" cloud,
	// i.e. the local LXD daemon. We may later want to detect
	// locally-configured remotes.
	switch name {
	case "lxd", "localhost":
		return localhostCloud, nil
	}
	return cloud.Cloud{}, errors.NotFoundf("cloud %s", name)
}

// FinalizeCloud is part of the environs.CloudFinalizer interface.
func (p *environProvider) FinalizeCloud(
	ctx environs.FinalizeCloudContext,
	in cloud.Cloud,
) (cloud.Cloud, error) {

	var endpoint string
	resolveEndpoint := func(name string, ep *string) error {
		// If the name doesn't equal "localhost" then we shouldn't resolve
		// the end point, instead we should just accept what we already have.
		if name != "localhost" || *ep != "" {
			return nil
		}
		if endpoint == "" {
			// The cloud endpoint is empty, which means
			// that we should connect to the local LXD.
			var err error
			hostAddress, err := p.getLocalHostAddress(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			endpoint = hostAddress
		}
		*ep = endpoint
		return nil
	}

	if err := resolveEndpoint(in.Name, &in.Endpoint); err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	for i, k := range in.Regions {
		if err := resolveEndpoint(k.Name, &in.Regions[i].Endpoint); err != nil {
			return cloud.Cloud{}, errors.Trace(err)
		}
	}
	return in, nil
}

func (p *environProvider) getLocalHostAddress(ctx environs.FinalizeCloudContext) (string, error) {
	svr, err := p.serverFactory.LocalServer()
	if err != nil {
		return "", errors.Trace(err)
	}

	bridgeName := svr.LocalBridgeName()
	hostAddress, err := p.serverFactory.LocalServerAddress()
	if err != nil {
		return "", errors.Trace(err)
	}
	ctx.Verbosef(
		"Resolved LXD host address on bridge %s: %s",
		bridgeName, hostAddress,
	)
	return hostAddress, nil
}

// localhostCloud is the predefined "localhost" LXD cloud. We leave the
// endpoints empty to indicate that LXD is on the local host. When the
// cloud is finalized (see FinalizeCloud), we resolve the bridge address
// of the LXD host, and use that as the endpoint.
var localhostCloud = cloud.Cloud{
	Name: lxdnames.DefaultCloud,
	Type: lxdnames.ProviderType,
	AuthTypes: []cloud.AuthType{
		interactiveAuthType,
		cloud.CertificateAuthType,
	},
	Endpoint: "",
	Regions: []cloud.Region{{
		Name:     lxdnames.DefaultRegion,
		Endpoint: "",
	}},
	Description: cloud.DefaultCloudDescription(lxdnames.ProviderType),
}

// DetectRegions implements environs.CloudRegionDetector.
func (*environProvider) DetectRegions() ([]cloud.Region, error) {
	// For now we just return a hard-coded "localhost" region,
	// i.e. the local LXD daemon. We may later want to detect
	// locally-configured remotes.
	return []cloud.Region{{Name: lxdnames.DefaultRegion}}, nil
}

// Schema returns the configuration schema for an environment.
func (*environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

func (p *environProvider) validateCloudSpec(spec environs.CloudSpec) (local bool, _ error) {
	if err := spec.Validate(); err != nil {
		return false, errors.Trace(err)
	}
	if spec.Credential == nil {
		return false, errors.NotValidf("missing credential")
	}
	if spec.Endpoint == "" {
		// If we're dealing with an old controller, or we're preparing
		// a local LXD, we'll have an empty endpoint. Connect to the
		// default Unix socket.
		local = true
	} else {
		if _, err := endpointURL(spec.Endpoint); err != nil {
			return false, errors.Trace(err)
		}
	}
	switch authType := spec.Credential.AuthType(); authType {
	case cloud.CertificateAuthType, interactiveAuthType:
		if _, _, ok := getCertificates(spec); !ok {
			return false, errors.NotValidf("certificate credentials")
		}
	default:
		return false, errors.NotSupportedf("%q auth-type", authType)
	}
	return local, nil
}

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p *environProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p *environProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}
