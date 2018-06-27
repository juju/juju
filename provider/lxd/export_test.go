// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"net"

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
		providerCredentials: creds,
		interfaceAddress:    interfaceAddress,
		newLocalServer:      newLocalSever,
	}
}

func NewProviderCredentials(
	generateMemCert func(bool) ([]byte, []byte, error),
	lookupHost func(string) ([]string, error),
	interfaceAddrs func() ([]net.Addr, error),
	newLocalServer func() (ProviderLXDServer, error),
) environs.ProviderCredentials {
	return environProviderCredentials{
		generateMemCert: generateMemCert,
		lookupHost:      lookupHost,
		interfaceAddrs:  interfaceAddrs,
		newLocalServer:  newLocalServer,
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
