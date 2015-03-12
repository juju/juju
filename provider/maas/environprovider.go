// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"launchpad.net/gomaasapi"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

// Logger for the MAAS provider.
var logger = loggo.GetLogger("juju.provider.maas")

type maasEnvironProvider struct{}

var _ environs.EnvironProvider = (*maasEnvironProvider)(nil)

var providerInstance maasEnvironProvider

func (maasEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Debugf("opening environment %q.", cfg.Name())
	env, err := NewEnviron(cfg)
	if err != nil {
		return nil, err
	}
	return env, nil
}

var errAgentNameAlreadySet = errors.New(
	"maas-agent-name is already set; this should not be set by hand")

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (p maasEnvironProvider) RestrictedConfigAttributes() []string {
	return []string{"maas-server"}
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (p maasEnvironProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	attrs := cfg.UnknownAttrs()
	oldName, found := attrs["maas-agent-name"]
	if found && oldName != "" {
		return nil, errAgentNameAlreadySet
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	attrs["maas-agent-name"] = uuid.String()
	return cfg.Apply(attrs)
}

func (p maasEnvironProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	cfg, err := p.PrepareForCreateEnvironment(cfg)
	if err != nil {
		return nil, err
	}
	env, err := p.Open(cfg)
	if err != nil {
		return nil, err
	}
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env.(*maasEnviron)); err != nil {
			return nil, err
		}
	}
	return env, nil
}

func verifyCredentials(env *maasEnviron) error {
	// Verify we can connect to the server and authenticate.
	_, err := env.getMAASClient().GetSubObject("maas").CallGet("get_config", nil)
	if err, ok := err.(gomaasapi.ServerError); ok && err.StatusCode == http.StatusUnauthorized {
		logger.Debugf("authentication failed: %v", err)
		return errors.New(`authentication failed.

Please ensure the credentials are correct.`)
	}
	return nil
}

// Boilerplate config YAML.  Don't mess with the indentation or add newlines!
var boilerplateYAML = `
# https://juju.ubuntu.com/docs/config-maas.html
maas:
    type: maas

    # maas-server specifies the location of the MAAS server. It must
    # specify the base path.
    #
    maas-server: 'http://192.168.1.1/MAAS/'

    # maas-oauth holds the OAuth credentials from MAAS.
    #
    maas-oauth: '<add your OAuth credentials from MAAS here>'

    # maas-server bootstrap ssh connection options
    #

    # bootstrap-timeout time to wait contacting a state server, in seconds.
    bootstrap-timeout: 1800

    # Whether or not to refresh the list of available updates for an
    # OS. The default option of true is recommended for use in
    # production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-refresh-update: true

    # Whether or not to perform OS upgrades when machines are
    # provisioned. The default option of true is recommended for use
    # in production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-upgrade: true


`[1:]

// BoilerplateConfig is specified in the EnvironProvider interface.
func (maasEnvironProvider) BoilerplateConfig() string {
	return boilerplateYAML
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov maasEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	secretAttrs := make(map[string]string)
	maasCfg, err := prov.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	secretAttrs["maas-oauth"] = maasCfg.maasOAuth()
	return secretAttrs, nil
}
