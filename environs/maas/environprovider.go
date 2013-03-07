package maas

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
)

type maasEnvironProvider struct{}

var _ environs.EnvironProvider = (*maasEnvironProvider)(nil)

var providerInstance maasEnvironProvider

func init() {
	environs.RegisterProvider("maas", &providerInstance)
}

func (*maasEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	log.Printf("environs/maas: opening environment %q.", cfg.Name())
	return NewEnviron(cfg)
}

// BoilerplateConfig is specified in the EnvironProvider interface.
func (*maasEnvironProvider) BoilerplateConfig() string {
	panic("Not implemented.")
}

// SecretAttrs is specified in the EnvironProvider interface.
func (*maasEnvironProvider) SecretAttrs(*config.Config) (map[string]interface{}, error) {
	panic("Not implemented.")
}

// PublicAddress is specified in the EnvironProvider interface.
func (*maasEnvironProvider) PublicAddress() (string, error) {
	panic("Not implemented.")
}

// PrivateAddress is specified in the EnvironProvider interface.
func (*maasEnvironProvider) PrivateAddress() (string, error) {
	panic("Not implemented.")
}

// InstanceId is specified in the EnvironProvider interface.
func (*maasEnvironProvider) InstanceId() (state.InstanceId, error) {
	panic("Not implemented.")
}
