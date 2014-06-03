package juju

import (
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/state/api"
)

var (
	ProviderConnectDelay = &providerConnectDelay
	GetConfig            = getConfig
)

type APIState apiState

type APIOpenFunc func(*api.Info, api.DialOpts) (APIState, error)

func NewAPIFromStore(envName string, store configstore.Storage, f APIOpenFunc) (APIState, error) {
	apiOpen := func(info *api.Info, opts api.DialOpts) (apiState, error) {
		return f(info, opts)
	}
	return newAPIFromStore(envName, store, apiOpen)
}

func APIEndpointInStore(envName string, refresh bool, store configstore.Storage, f APIOpenFunc) (configstore.APIEndpoint, error) {
	apiOpen := func(info *api.Info, opts api.DialOpts) (apiState, error) {
		return f(info, opts)
	}
	return apiEndpointInStore(envName, refresh, store, apiOpen)
}
