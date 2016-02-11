package juju

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
)

var (
	ProviderConnectDelay   = &providerConnectDelay
	GetConfig              = getConfig
	CacheChangedAPIInfo    = cacheChangedAPIInfo
	CacheAPIInfo           = cacheAPIInfo
	EnvironInfoUserTag     = environInfoUserTag
	MaybePreferIPv6        = &maybePreferIPv6
	ResolveOrDropHostnames = &resolveOrDropHostnames
	ServerAddress          = &serverAddress
)

func NewAPIFromStore(envName string, store configstore.Storage, controllerStore jujuclient.ControllerStore, f api.OpenFunc) (api.Connection, error) {
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return f(info, opts)
	}
	return newAPIFromStore(envName, store, controllerStore, apiOpen, nil)
}
