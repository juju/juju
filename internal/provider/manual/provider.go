// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
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
	err := initUbuntuUser(host, user, cfg.AuthorizedKeys(), "", ctx.GetStdin(), ctx.GetStdout())
	if err != nil {
		logger.Errorf(ctx, "initializing ubuntu user: %v", err)
		return err
	}
	logger.Infof(ctx, "initialized ubuntu user")
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
		cloud.EndpointKey: {
			Singular: "the ssh connection string for controller, username@<hostname or IP> or <hostname or IP>",
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
func (p ManualProvider) Ping(_ context.Context, endpoint string) error {
	if p.ping != nil {
		return p.ping(endpoint)
	}
	return pingMachine(endpoint)
}

var echo = func(s string) error {
	logger.Infof(context.Background(), "trying to ssh using %q", s)
	command := ssh.Command(s, []string{"echo", "hi"}, nil)
	// os/exec just returns an error that contains the error code from the
	// executable, which is basically useless, but stderr usually shows
	// something useful, so we show that instead.
	buf := bytes.Buffer{}
	command.Stderr = &buf
	if err := command.Run(); err != nil {
		if buf.Len() > 0 {
			return errors.New(buf.String())
		}
		return err
	}
	return nil
}

// pingMachine is what is used in production by ManualProvider.Ping().
// It does nothing at the moment.
func pingMachine(endpoint string) error {
	// There's no "just connect" command for utils/ssh, so we run a command that
	// should always work.
	// If "endpoint" contains connection user, test it as-is.
	if strings.Contains(endpoint, "@") {
		return echo(endpoint)
	}
	// Try using "endpoint" directly - it is either an IP address or a hostname
	// and as such ssh Command will try using the user name of the user from the current client device.
	if err := echo(endpoint); err != nil {
		// If it fails, try to use ubuntu user, lp#1649721.
		return echo(fmt.Sprintf("ubuntu@%v", endpoint))
	}
	return nil
}

// ValidateCloud is specified in the EnvironProvider interface.
func (ManualProvider) ValidateCloud(ctx context.Context, spec environscloudspec.CloudSpec) error {
	return errors.Annotate(validateCloudSpec(spec), "validating cloud spec")
}

// Version is part of the EnvironProvider interface.
func (ManualProvider) Version() int {
	return 0
}

func (p ManualProvider) Open(ctx context.Context, args environs.OpenParams, invaliator environs.CredentialInvalidator) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Trace(err)
	}
	_, err := p.validate(ctx, args.Config, nil)
	if err != nil {
		return nil, err
	}
	// validate adds missing manual-specific config attributes
	// with their defaults in the result; we don't want that in
	// Open.
	envConfig := newModelConfig(args.Config, args.Config.UnknownAttrs())
	host, user := args.Cloud.Endpoint, ""
	if i := strings.IndexRune(host, '@'); i >= 0 {
		user, host = host[:i], host[i+1:]
	}
	return p.open(ctx, host, user, envConfig)
}

func validateCloudSpec(spec environscloudspec.CloudSpec) error {
	if spec.Endpoint == "" {
		return errors.Errorf(
			"missing address of host to bootstrap: " +
				`please specify "juju bootstrap manual/[user@]<host>"`,
		)
	}
	return nil
}

func (p ManualProvider) open(ctx context.Context, host, user string, cfg *environConfig) (environs.Environ, error) {
	env := &manualEnviron{host: host, user: user, cfg: cfg}
	// Need to call SetConfig to initialise storage.
	if err := env.SetConfig(ctx, cfg.Config); err != nil {
		return nil, err
	}
	return env, nil
}

func (p ManualProvider) validate(ctx context.Context, cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(ctx, cfg, old); err != nil {
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
			logger.Infof(ctx, "%s was not defined. Defaulting to %v.", keyName, value)
			envConfig.attrs[keyName] = value
		}
	}

	// If the user hasn't specified a value, refresh the
	// available updates, but don't upgrade.
	defineIfNot(config.EnableOSRefreshUpdateKey, true)
	defineIfNot(config.EnableOSUpgradeKey, false)

	return envConfig, nil
}

func (p ManualProvider) Validate(ctx context.Context, cfg, old *config.Config) (valid *config.Config, err error) {
	envConfig, err := p.validate(ctx, cfg, old)
	if err != nil {
		return nil, err
	}
	return cfg.Apply(envConfig.attrs)
}
