package juju

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

var (
	ProviderConnectDelay   = &providerConnectDelay
	ResolveOrDropHostnames = &resolveOrDropHostnames
	ServerAddress          = &serverAddress
)

func NewAPIFromStore(
	controllerName, accountName, modelName string,
	store jujuclient.ClientStore,
	open api.OpenFunc,
	getBootstrapConfig func(controllerName string) (*config.Config, error),
) (api.Connection, error) {
	return newAPIFromStore(controllerName, accountName, modelName, store, open, nil, getBootstrapConfig)
}
