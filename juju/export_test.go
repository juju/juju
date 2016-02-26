package juju

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
)

var (
	ProviderConnectDelay   = &providerConnectDelay
	GetBootstrapConfig     = getBootstrapConfig
	MaybePreferIPv6        = &maybePreferIPv6
	ResolveOrDropHostnames = &resolveOrDropHostnames
	ServerAddress          = &serverAddress
)

func NewAPIFromStore(controllerName, accountName, modelName string, store configstore.Storage, controllerStore jujuclient.ClientStore, f api.OpenFunc) (api.Connection, error) {
	return newAPIFromStore(controllerName, accountName, modelName, store, controllerStore, f, nil)
}
