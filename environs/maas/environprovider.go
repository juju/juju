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

func (*maasEnvironProvider) SecretAttrs(*config.Config) (map[string]interface{}, error) {
	panic("Not implemented.")
}

func (*maasEnvironProvider) PublicAddress() (string, error) {
	panic("Not implemented.")
}

func (*maasEnvironProvider) PrivateAddress() (string, error) {
	panic("Not implemented.")
}
