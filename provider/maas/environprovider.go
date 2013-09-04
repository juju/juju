// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"os"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
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

func (p maasEnvironProvider) Prepare(cfg *config.Config) (environs.Environ, error) {
	// TODO any attributes to prepare?
	return p.Open(cfg)
}

// Boilerplate config YAML.  Don't mess with the indentation or add newlines!
const boilerplateYAML = `maas:
  type: maas
  # Change this to where your MAAS server lives.  It must specify the base path.
  maas-server: 'http://192.168.1.1/MAAS/'
  maas-oauth: '<add your OAuth credentials from MAAS here>'
  admin-secret: {{rand}}
  default-series: precise
  authorized-keys-path: ~/.ssh/authorized_keys # or any file you want.
  # Or:
  # authorized-keys: ssh-rsa keymaterialhere

`

// BoilerplateConfig is specified in the EnvironProvider interface.
func (maasEnvironProvider) BoilerplateConfig() string {
	return boilerplateYAML
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov maasEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	secretAttrs := make(map[string]interface{})
	maasCfg, err := prov.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	secretAttrs["maas-oauth"] = maasCfg.maasOAuth()
	return secretAttrs, nil
}

func (maasEnvironProvider) hostname() (string, error) {
	// Hack to get container ip addresses properly for MAAS demo.
	if os.Getenv(osenv.JujuContainerType) == string(instance.LXC) {
		return utils.GetAddressForInterface("eth0")
	}
	info := machineInfo{}
	err := info.load()
	if err != nil {
		return "", err
	}
	return info.Hostname, nil
}

// PublicAddress is specified in the EnvironProvider interface.
func (prov maasEnvironProvider) PublicAddress() (string, error) {
	return prov.hostname()
}

// PrivateAddress is specified in the EnvironProvider interface.
func (prov maasEnvironProvider) PrivateAddress() (string, error) {
	return prov.hostname()
}
