// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/schema"
	ociIdentity "github.com/oracle/oci-go-sdk/v65/identity"
	"gopkg.in/ini.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
	internallogger "github.com/juju/juju/internal/logger"
	providercommon "github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/oci/common"
)

var logger = internallogger.GetLogger("juju.provider.oci")

// EnvironProvider type implements environs.EnvironProvider interface
type EnvironProvider struct {
	ControllerUUID string
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

var _ config.ConfigSchemaSource = (*EnvironProvider)(nil)
var _ environs.ProviderSchema = (*EnvironProvider)(nil)

var configSchema = configschema.Fields{
	"compartment-id": {
		Description: "The OCID of the compartment in which juju has access to create resources.",
		Type:        configschema.Tstring,
	},
	"address-space": {
		Description: "The CIDR block to use when creating default subnets. The subnet must have at least a /16 size.",
		Type:        configschema.Tstring,
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
		// Deprecated, but still supported for backward compatibility
		{
			"region", cloud.CredentialAttr{
				Description: "DEPRECATED: Region to log into",
			},
		},
	},
}

func (p EnvironProvider) newConfig(ctx context.Context, cfg *config.Config) (*environConfig, error) {
	if cfg == nil {
		return nil, errors.New("cannot set config on uninitialized env")
	}

	valid, err := p.Validate(ctx, cfg, nil)
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
func (o *EnvironProvider) Schema() configschema.Fields {
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
	return nil
}

// Ping implements environs.EnvironProvider.
func (e *EnvironProvider) Ping(_ context.Context, _ string) error {
	return errors.NotImplementedf("Ping")
}

func validateCloudSpec(c environscloudspec.CloudSpec) error {
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

// ModelConfigDefaults provides a set of default model config attributes that
// should be set on a models config if they have not been specified by the user.
func (e EnvironProvider) ModelConfigDefaults(_ context.Context) (map[string]any, error) {
	return nil, nil
}

// ValidateCloud is specified in the EnvironProvider interface.
func (e EnvironProvider) ValidateCloud(ctx context.Context, spec environscloudspec.CloudSpec) error {
	return errors.Annotate(validateCloudSpec(spec), "validating cloud spec")
}

// Open implements environs.EnvironProvider.
func (e *EnvironProvider) Open(ctx context.Context, params environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
	logger.Infof(ctx, "opening model %q", params.Config.Name())

	if err := validateCloudSpec(params.Cloud); err != nil {
		return nil, errors.Trace(err)
	}

	creds := params.Cloud.Credential.Attributes()
	providerConfig := common.JujuConfigProvider{
		Key:         []byte(creds["key"]),
		Fingerprint: creds["fingerprint"],
		Passphrase:  creds["pass-phrase"],
		Tenancy:     creds["tenancy"],
		User:        creds["user"],
		OCIRegion:   params.Cloud.Region,
	}
	// We don't support setting a default region in the credentials anymore. Because, such approach conflicts with the
	// way we handle regions in Juju.
	if creds["region"] != "" {
		logger.Warningf(ctx, "Setting a default region in Oracle Cloud credentials is not supported.")
	}
	err := providerConfig.Validate()
	if err != nil {
		return nil, errors.Trace(err)
	}
	compute, err := common.NewComputeClient(providerConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	networking, err := common.NewNetworkClient(providerConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	storage, err := common.NewStorageClient(providerConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	identity, err := ociIdentity.NewIdentityClientWithConfigurationProvider(providerConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	env := &Environ{
		CredentialInvalidator: providercommon.NewCredentialInvalidator(invalidator, common.IsAuthorisationFailure),
		Compute:               compute,
		Networking:            networking,
		Storage:               storage,
		Firewall:              networking,
		Identity:              identity,
		ociConfig:             providerConfig,
		clock:                 clock.WallClock,
		controllerUUID:        params.ControllerUUID,
		p:                     e,
	}

	if err := env.SetConfig(ctx, params.Config); err != nil {
		return nil, err
	}

	env.namespace, err = instance.NewNamespace(env.Config().UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

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
func (e EnvironProvider) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
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

	for _, val := range cfg.SectionStrings() {
		values := new(credentialSection)
		if err := cfg.Section(val).MapTo(values); err != nil {
			logger.Warningf(context.TODO(), "invalid value in section %s: %s", val, err)
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

		if len(missingFields) > 0 {
			logger.Warningf(context.TODO(), "missing required field(s) in section %s: %s", val, strings.Join(missingFields, ", "))
			continue
		}

		pemFileContent, err := os.ReadFile(values.KeyFile)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := common.ValidateKey(pemFileContent, values.PassPhrase); err != nil {
			logger.Warningf(context.TODO(), "failed to decrypt PEM %s using the configured pass phrase", values.KeyFile)
			continue
		}

		httpSigCreds := cloud.NewCredential(
			cloud.HTTPSigAuthType,
			map[string]string{
				"user":        values.User,
				"tenancy":     values.Tenancy,
				"key":         string(pemFileContent),
				"pass-phrase": values.PassPhrase,
				"fingerprint": values.Fingerprint,
			},
		)
		httpSigCreds.Label = fmt.Sprintf("OCI credential %q", val)
		result.AuthCredentials[val] = httpSigCreds
	}
	if len(result.AuthCredentials) == 0 {
		return nil, errors.NotFoundf("OCI credentials")
	}
	return &result, nil
}

// FinalizeCredential implements environs.ProviderCredentials.
func (e EnvironProvider) FinalizeCredential(
	ctx environs.FinalizeCredentialContext,
	params environs.FinalizeCredentialParams) (*cloud.Credential, error) {

	return &params.Credential, nil
}

// Validate implements config.Validator.
func (e EnvironProvider) Validate(ctx context.Context, cfg, old *config.Config) (valid *config.Config, err error) {
	if err := config.Validate(ctx, cfg, old); err != nil {
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
