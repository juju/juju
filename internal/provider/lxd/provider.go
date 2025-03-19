// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/schema"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/container/lxd"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/provider/lxd/lxdnames"
)

// LXCConfigReader reads files required for the LXC configuration.
type LXCConfigReader interface {
	// ReadConfig takes a path and returns a LXCConfig.
	// Returns an error if there is an error with the location of the config
	// file, or there was an error parsing the file.
	ReadConfig(path string) (LXCConfig, error)

	// ReadCert takes a path and returns a raw certificate, there is no
	// validation of the certificate.
	// Returns an error if there is an error with the location of the
	// certificate.
	ReadCert(path string) ([]byte, error)
}

// LXCConfig represents a configuration setup of a LXC configuration file.
// The LXCConfig expects the configuration file to be in a yaml representation.
type LXCConfig struct {
	DefaultRemote string                     `yaml:"local"`
	Remotes       map[string]LXCRemoteConfig `yaml:"remotes"`
}

// LXCRemoteConfig defines a the remote servers of a LXC configuration.
type LXCRemoteConfig struct {
	Addr     string `yaml:"addr"`
	Public   bool   `yaml:"public"`
	Protocol string `yaml:"protocol"`
	AuthType string `yaml:"auth_type"`
}

type environProvider struct {
	environs.ProviderCredentials
	environs.RequestFinalizeCredential
	environs.ProviderCredentialsRegister
	serverFactory   ServerFactory
	lxcConfigReader LXCConfigReader
	Clock           clock.Clock
}

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey, cloud.AuthTypesKey},
	Order:    []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: {
			Singular: "the API endpoint url for the remote LXD server",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
		cloud.AuthTypesKey: {
			Singular:    "auth type",
			Plural:      "auth types",
			Type:        []jsonschema.Type{jsonschema.ArrayType},
			UniqueItems: jsonschema.Bool(true),
			Default:     string(cloud.CertificateAuthType),
			Items: &jsonschema.ItemSpec{
				Schemas: []*jsonschema.Schema{{
					Type: []jsonschema.Type{jsonschema.StringType},
					Enum: []interface{}{
						string(cloud.CertificateAuthType),
					},
				}},
			},
		},
		cloud.RegionsKey: {
			Type:     []jsonschema.Type{jsonschema.ObjectType},
			Singular: "region",
			Plural:   "regions",
			Default:  "default",
			AdditionalProperties: &jsonschema.Schema{
				Type:          []jsonschema.Type{jsonschema.ObjectType},
				Required:      []string{cloud.EndpointKey},
				MaxProperties: jsonschema.Int(1),
				Properties: map[string]*jsonschema.Schema{
					cloud.EndpointKey: {
						Singular:      "the API endpoint url for the region",
						Type:          []jsonschema.Type{jsonschema.StringType},
						Format:        jsonschema.FormatURI,
						Default:       "",
						PromptDefault: "use cloud api url",
					},
				},
			},
		},
	},
}

// NewProvider returns a new LXD EnvironProvider.
func NewProvider() environs.CloudEnvironProvider {
	configReader := lxcConfigReader{}
	factory := NewServerFactory(NewHTTPClientFunc(func() *http.Client {
		return jujuhttp.NewClient(
			jujuhttp.WithLogger(logger.Child("http", corelogger.HTTP)),
		).Client()
	}))

	credentials := environProviderCredentials{
		certReadWriter:  certificateReadWriter{},
		certGenerator:   certificateGenerator{},
		serverFactory:   factory,
		lxcConfigReader: configReader,
	}
	return &environProvider{
		ProviderCredentials:         credentials,
		RequestFinalizeCredential:   credentials,
		ProviderCredentialsRegister: credentials,
		serverFactory:               factory,
		lxcConfigReader:             configReader,
	}
}

// Version is part of the EnvironProvider interface.
func (*environProvider) Version() int {
	return 0
}

// Open implements environs.EnvironProvider.
func (p *environProvider) Open(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
	if err := p.validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(
		ctx,
		p,
		args.Cloud,
		args.Config,
		invalidator,
	)
	return env, errors.Trace(err)
}

// CloudSchema returns the schema used to validate input for add-cloud.
func (p *environProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p *environProvider) Ping(_ context.Context, endpoint string) error {
	// if the endpoint is empty, then don't ping, as we can assume we're using
	// local lxd
	if endpoint == "" {
		return nil
	}

	// Ensure the Port on the Host, if we get an error it is reasonable to
	// assume that the address in the spec is invalid.
	lxdEndpoint, err := lxd.EnsureHostPort(endpoint)
	if err != nil {
		return errors.Trace(err)
	}

	// Connect to the remote server anonymously so we can just verify it exists
	// as we're not sure that the certificates are loaded in time for when the
	// ping occurs i.e. interactive add-cloud
	_, err = lxd.ConnectRemote(lxd.NewInsecureServerSpec(lxdEndpoint))
	if err != nil {
		return errors.Annotatef(err, "no lxd server running at %s", lxdEndpoint)
	}
	return nil
}

// ModelConfigDefaults provides a set of default model config attributes that
// should be set on a models config if they have not been specified by the user.
func (p *environProvider) ModelConfigDefaults(_ context.Context) (map[string]any, error) {
	return map[string]any{
		config.StorageDefaultFilesystemSourceKey: lxdStorageProviderType,
	}, nil
}

// ValidateCloud is specified in the EnvironProvider interface.
func (p *environProvider) ValidateCloud(ctx context.Context, spec environscloudspec.CloudSpec) error {
	return errors.Annotate(p.validateCloudSpec(spec), "validating cloud spec")
}

// Validate implements environs.EnvironProvider.
func (*environProvider) Validate(ctx context.Context, cfg, old *config.Config) (valid *config.Config, err error) {
	if _, err := newValidConfig(ctx, cfg); err != nil {
		return nil, errors.Annotate(err, "invalid base config")
	}
	return cfg, nil
}

// DetectClouds implements environs.CloudDetector.
func (p *environProvider) DetectClouds() ([]cloud.Cloud, error) {
	usedNames := map[string]bool{lxdnames.ProviderType: true, lxdnames.DefaultCloud: true}
	clouds := []cloud.Cloud{localhostCloud}
	for _, dir := range configDirs() {
		configPath := filepath.Join(dir, "config.yml")
		config, err := p.lxcConfigReader.ReadConfig(configPath)
		if err != nil {
			if !strings.HasSuffix(err.Error(), "permission denied") {
				logger.Warningf(context.TODO(), "unable to read/parse LXC config file (%s): %s", configPath, err)
			}
		}
		for name, remote := range config.Remotes {
			if remote.Protocol != lxdnames.ProviderType {
				continue
			}
			if usedNames[name] {
				logger.Warningf(context.TODO(), "ignoring ambigious cloud %s", name)
				continue
			}
			usedNames[name] = true
			clouds = append(clouds, cloud.Cloud{
				Name:        name,
				Type:        lxdnames.ProviderType,
				Endpoint:    remote.Addr,
				Description: "LXD Cluster",
				AuthTypes: []cloud.AuthType{
					cloud.CertificateAuthType,
				},
				Regions: []cloud.Region{{
					Name:     lxdnames.DefaultRemoteRegion,
					Endpoint: remote.Addr,
				}},
			})
		}
	}
	return clouds, nil
}

// DetectCloud implements environs.CloudDetector.
func (p *environProvider) DetectCloud(name string) (cloud.Cloud, error) {
	clouds, err := p.DetectClouds()
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	match := name
	if match == lxdnames.DefaultCloudAltName {
		match = lxdnames.DefaultCloud
	}
	for _, cloud := range clouds {
		if cloud.Name == match {
			return cloud, nil
		}
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
		if !lxdnames.IsDefaultCloud(name) || *ep != "" {
			return nil
		}
		if endpoint == "" {
			// The cloud endpoint is empty, which means
			// that we should connect to the local LXD.
			hostAddress, err := getLocalHostAddress(ctx, p.serverFactory)
			if err != nil {
				return errors.Trace(err)
			}
			endpoint = hostAddress
		}
		*ep = endpoint
		return nil
	}

	// If any of the endpoints point to localhost, go through and backfill the
	// cloud spec with local host addresses.
	if err := resolveEndpoint(in.Name, &in.Endpoint); err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	for i, k := range in.Regions {
		if err := resolveEndpoint(k.Name, &in.Regions[i].Endpoint); err != nil {
			return cloud.Cloud{}, errors.Trace(err)
		}
	}
	// If the provider type is not named localhost and there is no region, set the
	// region to be a default region
	if !lxdnames.IsDefaultCloud(in.Name) && len(in.Regions) == 0 {
		in.Regions = append(in.Regions, cloud.Region{
			Name:     lxdnames.DefaultRemoteRegion,
			Endpoint: in.Endpoint,
		})
	}
	return in, nil
}

func getLocalHostAddress(ctx environs.FinalizeCloudContext, serverFactory ServerFactory) (string, error) {
	svr, err := serverFactory.LocalServer()
	if err != nil {
		return "", errors.Trace(err)
	}

	bridgeName := svr.LocalBridgeName()
	hostAddress, err := serverFactory.LocalServerAddress()
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
		cloud.CertificateAuthType,
	},
	Endpoint: "",
	Regions: []cloud.Region{{
		Name:     lxdnames.DefaultLocalRegion,
		Endpoint: "",
	}},
	Description: cloud.DefaultCloudDescription(lxdnames.ProviderType),
}

// DetectRegions implements environs.CloudRegionDetector.
func (*environProvider) DetectRegions() ([]cloud.Region, error) {
	// For now we just return a hard-coded "localhost" region,
	// i.e. the local LXD daemon. We may later want to detect
	// locally-configured remotes.
	return []cloud.Region{{Name: lxdnames.DefaultLocalRegion}}, nil
}

// Schema returns the configuration schema for an environment.
func (*environProvider) Schema() configschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

func (p *environProvider) validateCloudSpec(spec environscloudspec.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}

	// Always validate the spec.Endpoint, to ensure that it's valid.
	if _, err := endpointURL(spec.Endpoint); err != nil {
		return errors.Trace(err)
	}
	switch authType := spec.Credential.AuthType(); authType {
	case cloud.CertificateAuthType:
		if spec.Credential == nil {
			return errors.NotFoundf("credentials")
		}
		if _, _, ok := getCertificates(*spec.Credential); !ok {
			return errors.NotValidf("certificate credentials")
		}
	default:
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
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

// lxcConfigReader is the default implementation for reading files from disk.
type lxcConfigReader struct{}

func (lxcConfigReader) ReadConfig(path string) (LXCConfig, error) {
	configFile, err := os.ReadFile(path)
	if err != nil {
		if cause := errors.Cause(err); os.IsNotExist(cause) {
			return LXCConfig{}, nil
		}
		return LXCConfig{}, errors.Trace(err)
	}

	var config LXCConfig
	if err := yaml.Unmarshal(configFile, &config); err != nil {
		return LXCConfig{}, errors.Trace(err)
	}

	return config, nil
}

func (lxcConfigReader) ReadCert(path string) ([]byte, error) {
	certFile, err := os.ReadFile(path)
	return certFile, errors.Trace(err)
}
