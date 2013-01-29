package maas

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
)

type maasEnvironProvider struct {}

var _ environs.EnvironProvider = (*maasEnvironProvider)(nil)

func (*maasEnvironProvider) Open(*config.Config) (environs.Environ, error) {
	panic("Not implemented")
}

func (*maasEnvironProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	panic("Not implemented")
}

func (*maasEnvironProvider) SecretAttrs(*config.Config) (map[string]interface{}, error) {
	panic("Not implemented")
}

func (*maasEnvironProvider) PublicAddress() (string, error) {
	panic("Not implemented")
}

func (*maasEnvironProvider) PrivateAddress() (string, error) {
	panic("Not implemented")
}
