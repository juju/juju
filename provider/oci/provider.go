// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo"
	"github.com/juju/schema"
	ociCore "github.com/oracle/oci-go-sdk/core"
	ociIdentity "github.com/oracle/oci-go-sdk/identity"
	"gopkg.in/ini.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	providerCommon "github.com/juju/juju/provider/oci/common"
)

var logger = loggo.GetLogger("juju.provider.oci")

// EnvironProvider type implements environs.EnvironProvider interface
type EnvironProvider struct{}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

var _ config.ConfigSchemaSource = (*EnvironProvider)(nil)
var _ environs.ProviderSchema = (*EnvironProvider)(nil)

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.RegionsKey, cloud.AuthTypesKey},
	Order:    []string{cloud.RegionsKey, cloud.AuthTypesKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.RegionsKey: {
			Type:     []jsonschema.Type{jsonschema.ObjectType},
			Singular: "region",
			Plural:   "regions",
			AdditionalProperties: &jsonschema.Schema{
				Type: []jsonschema.Type{jsonschema.ObjectType},
			},
		},
		cloud.AuthTypesKey: {
			// don't need a prompt, since there's only one choice.
			Type: []jsonschema.Type{jsonschema.ArrayType},
			Enum: []interface{}{[]string{string(cloud.HTTPSigAuthType)}},
		},
	},
}

var configSchema = environschema.Fields{
	"compartment-id": {
		Description: "The OCID of the compartment in which juju has access to create resources.",
		Type:        environschema.Tstring,
	},
	"address-space": {
		Description: "The CIDR block to use when creating default subnets. The subnet must have at least a /16 size.",
		Type:        environschema.Tstring,
	},
}

var configDefaults = schema.Defaults{
	"compartment-id": "",
	"address-space":  DefaultAddressSpace,
}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

// credentialSection holds the keys present in one section of the OCI
// config file, as created by the OCI command line. This is only used
// during credential detection
type credentialSection struct {
	User        string
	Tenancy     string
	KeyFile     string
	PassPhrase  string
	Fingerprint string
	Region      string
}

var credentialSchema = map[cloud.AuthType]cloud.CredentialSchema{
	cloud.HTTPSigAuthType: {
		{
			"user", cloud.CredentialAttr{
				Description: "Username OCID",
			},
		},
		{
			"tenancy", cloud.CredentialAttr{
				Description: "Tenancy OCID",
			},
		},
		{
			"key", cloud.CredentialAttr{
				Description: "PEM encoded private key",
			},
		},
		{
			"pass-phrase", cloud.CredentialAttr{
				Description: "Passphrase used to unlock the key",
				Hidden:      true,
			},
		},
		{
			"fingerprint", cloud.CredentialAttr{
				Description: "Private key fingerprint",
			},
		},
		{
			"region", cloud.CredentialAttr{
				Description: "Region to log into",
			},
		},
	},
}

func (p EnvironProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	if cfg == nil {
		return nil, errors.New("cannot set config on uninitialized env")
	}

	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func (c *environConfig) compartmentID() *string {
	compartmentID := c.attrs["compartment-id"].(string)
	if compartmentID == "" {
		return nil
	}
	return &compartmentID
}

func (c *environConfig) addressSpace() *string {
	addressSpace := c.attrs["address-space"].(string)
	if addressSpace == "" {
		addressSpace = DefaultAddressSpace
	}
	return &addressSpace
}

// Schema implements environs.ProviderSchema
func (o *EnvironProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

// ConfigSchema implements config.ConfigSchemaSource
func (o *EnvironProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults implements config.ConfigSchemaSource
func (o *EnvironProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}

// Version implements environs.EnvironProvider.
func (e EnvironProvider) Version() int {
	return 1
}

// CloudSchema implements environs.EnvironProvider.
func (e EnvironProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping implements environs.EnvironProvider.
func (e *EnvironProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

func validateCloudSpec(c environs.CloudSpec) error {
	if err := c.Validate(); err != nil {
		return errors.Trace(err)
	}
	if c.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := c.Credential.AuthType(); authType != cloud.HTTPSigAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}

// PrepareConfig implements environs.EnvironProvider.
func (e EnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	// TODO(gsamfira): Set default block storage backend
	return args.Config, nil
}

// Open implements environs.EnvironProvider.
func (e *EnvironProvider) Open(params environs.OpenParams) (environs.Environ, error) {
	logger.Infof("opening model %q", params.Config.Name())

	if err := validateCloudSpec(params.Cloud); err != nil {
		return nil, errors.Trace(err)
	}

	creds := params.Cloud.Credential.Attributes()
	jujuConfig := providerCommon.JujuConfigProvider{
		Key:         []byte(creds["key"]),
		Fingerprint: creds["fingerprint"],
		Passphrase:  creds["pass-phrase"],
		Tenancy:     creds["tenancy"],
		User:        creds["user"],
		OCIRegion:   creds["region"],
	}
	provider, err := jujuConfig.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	compute, err := ociCore.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, errors.Trace(err)
	}

	networking, err := ociCore.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, errors.Trace(err)
	}

	storage, err := ociCore.NewBlockstorageClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, errors.Trace(err)
	}

	identity, err := ociIdentity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, errors.Trace(err)
	}

	env := &Environ{
		Compute:    compute,
		Networking: networking,
		Storage:    storage,
		Firewall:   networking,
		Identity:   identity,
		ociConfig:  provider,
		clock:      clock.WallClock,
		p:          e,
	}

	if err := env.SetConfig(params.Config); err != nil {
		return nil, err
	}

	env.namespace, err = instance.NewNamespace(env.Config().UUID())

	cfg := env.ecfg()
	if cfg.compartmentID() == nil {
		return nil, errors.New("compartment-id may not be empty")
	}

	addressSpace := cfg.addressSpace()
	if _, ipNET, err := net.ParseCIDR(*addressSpace); err == nil {
		size, _ := ipNET.Mask.Size()
		if size > 16 {
			return nil, errors.Errorf("configured subnet (%q) is not large enough. Please use a prefix length in the range /8 to /16. Current prefix length is /%d", *addressSpace, size)
		}
	} else {
		return nil, errors.Trace(err)
	}

	return env, nil
}

// CredentialSchemas implements environs.ProviderCredentials.
func (e EnvironProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return credentialSchema
}

// DetectCredentials implements environs.ProviderCredentials.
// Configuration options for the OCI SDK are detailed here:
// https://docs.us-phoenix-1.oraclecloud.com/Content/API/Concepts/sdkconfig.htm
func (e EnvironProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	result := cloud.CloudCredential{
		AuthCredentials: make(map[string]cloud.Credential),
	}
	cfg_file, err := ociConfigFile()
	if err != nil {
		if os.IsNotExist(errors.Cause(err)) {
			return &result, nil
		}
		return nil, errors.Trace(err)
	}

	cfg, err := ini.LooseLoad(cfg_file)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg.NameMapper = ini.TitleUnderscore

	var defaultRegion string

	for _, val := range cfg.SectionStrings() {
		values := new(credentialSection)
		if err := cfg.Section(val).MapTo(values); err != nil {
			logger.Warningf("invalid value in section %s: %s", val, err)
			continue
		}
		missingFields := []string{}
		if values.User == "" {
			missingFields = append(missingFields, "user")
		}

		if values.Tenancy == "" {
			missingFields = append(missingFields, "tenancy")
		}

		if values.KeyFile == "" {
			missingFields = append(missingFields, "key_file")
		}

		if values.Fingerprint == "" {
			missingFields = append(missingFields, "fingerprint")
		}
		if values.Region == "" {
			missingFields = append(missingFields, "region")
		}

		if len(missingFields) > 0 {
			logger.Warningf("missing required field(s) in section %s: %s", val, strings.Join(missingFields, ", "))
			continue
		}

		pemFileContent, err := ioutil.ReadFile(values.KeyFile)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := providerCommon.ValidateKey(pemFileContent, values.PassPhrase); err != nil {
			logger.Warningf("failed to decrypt PEM %s using the configured pass phrase", values.KeyFile)
			continue
		}

		if val == "DEFAULT" {
			defaultRegion = values.Region
		}
		httpSigCreds := cloud.NewCredential(
			cloud.HTTPSigAuthType,
			map[string]string{
				"user":        values.User,
				"tenancy":     values.Tenancy,
				"key":         string(pemFileContent),
				"pass-phrase": values.PassPhrase,
				"fingerprint": values.Fingerprint,
				"region":      values.Region,
			},
		)
		httpSigCreds.Label = fmt.Sprintf("OCI credential %q", val)
		result.AuthCredentials[val] = httpSigCreds
	}
	if len(result.AuthCredentials) == 0 {
		return nil, errors.NotFoundf("OCI credentials")
	}
	result.DefaultRegion = defaultRegion
	return &result, nil
}

// FinalizeCredential implements environs.ProviderCredentials.
func (e EnvironProvider) FinalizeCredential(
	ctx environs.FinalizeCredentialContext,
	params environs.FinalizeCredentialParams) (*cloud.Credential, error) {

	return &params.Credential, nil
}

// Validate implements config.Validator.
func (e EnvironProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	newAttrs, err := cfg.ValidateUnknownAttrs(
		configFields, configDefaults,
	)
	if err != nil {
		return nil, err
	}

	return cfg.Apply(newAttrs)
}
