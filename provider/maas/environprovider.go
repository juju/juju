// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"errors"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/utils"
)

// Logger for the MAAS provider.
var logger = loggo.GetLogger("juju.provider.maas")

type maasEnvironProvider struct{}

var _ environs.EnvironProvider = (*maasEnvironProvider)(nil)

var providerInstance maasEnvironProvider

func init() {
	environs.RegisterProvider("maas", maasEnvironProvider{})
}

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

func (p maasEnvironProvider) Prepare(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
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
	cfg, err = cfg.Apply(attrs)
	if err != nil {
		return nil, err
	}
	return p.Open(cfg)
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
