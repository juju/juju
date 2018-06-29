// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/juju/environs"

	"github.com/juju/juju/container/lxd"
)

var (
	NewInstance     = newInstance
	GetCertificates = getCertificates
)

func NewProviderWithMocks(
	creds environs.ProviderCredentials,
	interfaceAddress InterfaceAddress,
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

func ExposeEnvServer(env *environ) Server {
	return env.server
}

func GetImageSources(env *environ) ([]lxd.ServerSpec, error) {
	return env.getImageSources()
}
