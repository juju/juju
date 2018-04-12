// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual/sshprovisioner"
)

// ManualProvider contains the logic for using a random ubuntu machine as a
// controller, connected via SSH.
type ManualProvider struct {
	environProviderCredentials
	ping func(endpoint string) error
}

// Verify that we conform to the interface.
var _ environs.EnvironProvider = (*ManualProvider)(nil)

var initUbuntuUser = sshprovisioner.InitUbuntuUser

func ensureBootstrapUbuntuUser(ctx environs.BootstrapContext, host, user string, cfg *environConfig) error {
	err := initUbuntuUser(host, user, cfg.AuthorizedKeys(), ctx.GetStdin(), ctx.GetStdout())
	if err != nil {
		logger.Errorf("initializing ubuntu user: %v", err)
		return err
	}
	logger.Infof("initialized ubuntu user")
	return nil
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p ManualProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: &jsonschema.Schema{
			Singular: "the controller's hostname or IP address",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
	},
}

// CloudSchema returns the schema for verifying the cloud configuration.
func (p ManualProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p ManualProvider) Ping(endpoint string) error {
	if p.ping != nil {
		return p.ping(endpoint)
	}
	return pingMachine(endpoint)
}

// pingMachine is what is used in production by ManualProvider.Ping().
// It does nothing at the moment.
func pingMachine(endpoint string) error {
	// (anastasiamac 2017-03-30) This method was introduced to verify
	// manual endpoint by attempting to SSH into it.
	// However, what we really wanted to do was to determine if
	// we could connect to the endpoint not whether we could authenticate.
	// In other words, we wanted to ignore authentication errors.
	// These errors, at verification stage, when adding cloud details, are meaningless
	// since authentication is configurable at bootstrap.
	// With OpenSSH and crypto/ssh, both underlying current SSH client implementations, it is not
	// possible to cleanly distinguish between authentication and connection failures
	// without examining error string and looking for various matches.
	// This feels dirty and flaky as the error messages can easily change
	// between different libraries and their versions.
	// So, it has been decided to just accept endpoint.
	// If this ping(..) call will be used for other purposes, this decision may
	// need to be re-visited.
	return nil
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p ManualProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Trace(err)
	}
	envConfig, err := p.validate(args.Config, nil)
	if err != nil {
		return nil, err
	}
	return args.Config.Apply(envConfig.attrs)
}

// Version is part of the EnvironProvider interface.
func (ManualProvider) Version() int {
	return 0
}

func (p ManualProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Trace(err)
	}
	_, err := p.validate(args.Config, nil)
	if err != nil {
		return nil, err
	}
	// validate adds missing manual-specific config attributes
	// with their defaults in the result; we don't wnat that in
	// Open.
	envConfig := newModelConfig(args.Config, args.Config.UnknownAttrs())
	host, user := args.Cloud.Endpoint, ""
	if i := strings.IndexRune(host, '@'); i >= 0 {
		user, host = host[:i], host[i+1:]
	}
	return p.open(host, user, envConfig)
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if spec.Endpoint == "" {
		return errors.Errorf(
			"missing address of host to bootstrap: " +
				`please specify "juju bootstrap manual/[user@]<host>"`,
		)
	}
	return nil
}

func (p ManualProvider) open(host, user string, cfg *environConfig) (environs.Environ, error) {
	env := &manualEnviron{host: host, user: user, cfg: cfg}
	// Need to call SetConfig to initialise storage.
	if err := env.SetConfig(cfg.Config); err != nil {
		return nil, err
	}
	return env, nil
}

func (p ManualProvider) validate(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	envConfig := newModelConfig(cfg, validated)

	// If the user hasn't already specified a value, set it to the
	// given value.
	defineIfNot := func(keyName string, value interface{}) {
		if _, defined := cfg.AllAttrs()[keyName]; !defined {
			logger.Infof("%s was not defined. Defaulting to %v.", keyName, value)
			envConfig.attrs[keyName] = value
		}
	}

	// If the user hasn't specified a value, refresh the
	// available updates, but don't upgrade.
	defineIfNot("enable-os-refresh-update", true)
	defineIfNot("enable-os-upgrade", false)

	return envConfig, nil
}

func (p ManualProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	envConfig, err := p.validate(cfg, old)
	if err != nil {
		return nil, err
	}
	return cfg.Apply(envConfig.attrs)
}
