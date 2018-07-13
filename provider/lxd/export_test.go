// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/juju/environs"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/container/lxd"
)

var (
	NewInstance           = newInstance
	GetCertificates       = getCertificates
	IsSupportedAPIVersion = isSupportedAPIVersion
	ClientX509Cert        = clientX509Cert
)

func NewProviderWithMocks(
	creds environs.ProviderCredentials,
	serverFactory ServerFactory,
) environs.EnvironProvider {
	return &environProvider{
		ProviderCredentials: creds,
		serverFactory:       serverFactory,
	}
}

func NewProviderCredentials(
	certReadWriter CertificateReadWriter,
	certGenerator CertificateGenerator,
	lookup NetLookup,
	serverFactory ServerFactory,
) environs.ProviderCredentials {
	return environProviderCredentials{
		certReadWriter: certReadWriter,
		certGenerator:  certGenerator,
		lookup:         lookup,
		serverFactory:  serverFactory,
	}
}

func NewServerFactoryWithMocks(localServerFunc func() (Server, error),
	remoteServerFunc func(lxd.ServerSpec) (Server, error),
	interfaceAddress InterfaceAddress,
	clock clock.Clock,
) ServerFactory {
	return &serverFactory{
		newLocalServerFunc:  localServerFunc,
		newRemoteServerFunc: remoteServerFunc,
		interfaceAddress:    interfaceAddress,
		clock:               clock,
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
