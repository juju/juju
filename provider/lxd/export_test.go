// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"errors"
	"net/http"

	"github.com/juju/clock"

	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
)

var (
	NewInstance     = newInstance
	GetCertificates = getCertificates
	ParseAPIVersion = parseAPIVersion
)

func NewProviderWithMocks(
	creds environs.ProviderCredentials,
	credsRegister environs.ProviderCredentialsRegister,
	serverFactory ServerFactory,
	configReader LXCConfigReader,
) environs.EnvironProvider {
	return &environProvider{
		ProviderCredentials:         creds,
		ProviderCredentialsRegister: credsRegister,
		serverFactory:               serverFactory,
		lxcConfigReader:             configReader,
	}
}

func NewProviderCredentials(
	certReadWriter CertificateReadWriter,
	certGenerator CertificateGenerator,
	serverFactory ServerFactory,
	configReader LXCConfigReader,
) environs.ProviderCredentials {
	return environProviderCredentials{
		certReadWriter:  certReadWriter,
		certGenerator:   certGenerator,
		serverFactory:   serverFactory,
		lxcConfigReader: configReader,
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
		newHTTPClientFunc: NewHTTPClientFunc(func() *http.Client {
			return &http.Client{}
		}),
	}
}

func ExposeInstContainer(inst *environInstance) *lxd.Container {
	return inst.container
}

func ExposeInstEnv(inst *environInstance) *environ {
	return inst.env
}

func ExposeEnvConfig(env *environ) *environConfig {
	return env.ecfgUnlocked
}

func ExposeEnvServer(env *environ) Server {
	return env.server()
}

func GetImageSources(env environs.Environ) ([]lxd.ServerSpec, error) {
	lxdEnv, ok := env.(*environ)
	if !ok {
		return nil, errors.New("not a LXD environ")
	}
	return lxdEnv.getImageSources()
}
