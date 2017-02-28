// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/utils/ssh"

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

// pingMachine is what is used in production by ManualProvider.Ping().  It
// attempts a simplistic ssh connection to verify the machine exists and that
// you can log into it with SSH.
func pingMachine(endpoint string) error {
	// There's no "just connect" command for utils/ssh, so we run a command that
	// should always work.
	cmd := ssh.Command(endpoint, []string{"echo", "hi"}, nil)

	// os/exec just returns an error that contains the error code from the
	// executable, which is basically useless, but stderr usually shows
	// something useful, so we show that instead.
	buf := bytes.Buffer{}
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		if buf.Len() > 0 {
			received := buf.String()
			if strings.HasPrefix(received, "Permission denied") {
				// we have managed to reach the machine and just failed to authenticate.
				// consider this is a successful ping
				return nil
			}
			return errors.New(received)
		}
		return err
	}
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

func checkImmutableString(cfg, old *environConfig, key string) error {
	if old.attrs[key] != cfg.attrs[key] {
		return fmt.Errorf("cannot change %s from %q to %q", key, old.attrs[key], cfg.attrs[key])
	}
	return nil
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
