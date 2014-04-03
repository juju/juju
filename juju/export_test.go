package juju

import (
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/state/api"
)

var (
	ProviderConnectDelay = &providerConnectDelay
)

type APIState apiState

type APIOpenFunc func(*api.Info, api.DialOpts) (APIState, error)

func NewAPIFromStore(envName string, store configstore.Storage, f APIOpenFunc) (APIState, error) {
	apiOpen := func(info *api.Info, opts api.DialOpts) (apiState, error) {
		return f(info, opts)
	}
	return newAPIFromStore(envName, store, apiOpen)
}
