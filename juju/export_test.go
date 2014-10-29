package juju

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/environs/configstore"
)

var (
	ProviderConnectDelay = &providerConnectDelay
	GetConfig            = getConfig
	CacheChangedAPIInfo  = cacheChangedAPIInfo
	EnvironInfoUserTag   = environInfoUserTag
)

type APIState apiState

type APIOpenFunc func(*api.Info, api.DialOpts) (APIState, error)

func NewAPIFromStore(envName string, store configstore.Storage, f APIOpenFunc) (APIState, error) {
	apiOpen := func(info *api.Info, opts api.DialOpts) (apiState, error) {
		return f(info, opts)
	}
	return newAPIFromStore(envName, store, apiOpen)
}
