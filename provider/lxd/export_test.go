// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/juju/environs"

	"github.com/juju/juju/container/lxd"
)

var (
	NewInstance = newInstance
)

func NewProviderWithMocks(
	creds environs.ProviderCredentials,
	interfaceAddress func(string) (string, error),
	newLocalSever func() (ProviderLXDServer, error),
) environs.EnvironProvider {
	return &environProvider{
		ProviderCredentials: creds,
		interfaceAddress:    interfaceAddress,
		newLocalServer:      newLocalSever,
	}
}

func NewProviderCredentials(
	certReadWriter CertificateReadWriter,
	certGenerator CertificateGenerator,
	lookup NetLookup,
	newLocalServer func() (ProviderLXDServer, error),
) environs.ProviderCredentials {
	return environProviderCredentials{
		certReadWriter: certReadWriter,
		certGenerator:  certGenerator,
		lookup:         lookup,
		newLocalServer: newLocalServer,
	}
}

func ExposeInstContainer(inst *environInstance) *lxd.Container {
	return inst.container
}

func ExposeInstEnv(inst *environInstance) *environ {
	return inst.env
}

func ExposeEnvConfig(env *environ) *environConfig {
	return env.ecfg
}

func ExposeEnvServer(env *environ) newServer {
	return env.raw.newServer
}

func GetImageSources(env *environ) ([]lxd.RemoteServer, error) {
	return env.getImageSources()
}
