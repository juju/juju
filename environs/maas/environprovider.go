package maas

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
)

type maasEnvironProvider struct{}

var _ environs.EnvironProvider = (*maasEnvironProvider)(nil)

var providerInstance maasEnvironProvider

func init() {
	environs.RegisterProvider("maas", maasEnvironProvider{})
}

func (maasEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	log.Debugf("environs/maas: opening environment %q.", cfg.Name())
	return NewEnviron(cfg)
}

// BoilerplateConfig is specified in the EnvironProvider interface.
func (maasEnvironProvider) BoilerplateConfig() string {
	return `
  maas:
    type: maas
    # Change this to where your MAAS server lives.  It must specify the API endpoint.
    maas-server: 'http://192.168.1.1/MAAS/api/1.0'
    maas-oauth: '<add your OAuth credentials from MAAS here>'
    admin-secret: {{rand}}
    default-series: precise
    authorized-keys-path: ~/.ssh/authorized_keys # or any file you want.
    # Or:
    # authorized-keys: ssh-rsa keymaterialhere
`[1:]
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov maasEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	secretAttrs := make(map[string]interface{})
	maasCfg, err := prov.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	secretAttrs["maas-oauth"] = maasCfg.MAASOAuth()
	return secretAttrs, nil
}

func (maasEnvironProvider) hostname() (string, error) {
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

// InstanceId is specified in the EnvironProvider interface.
func (maasEnvironProvider) InstanceId() (state.InstanceId, error) {
	info := machineInfo{}
	err := info.load()
	if err != nil {
		return "", err
	}
	return state.InstanceId(info.InstanceId), nil
}
