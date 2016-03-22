package juju

import "github.com/juju/juju/api"

var (
	ProviderConnectDelay   = &providerConnectDelay
	ResolveOrDropHostnames = &resolveOrDropHostnames
	ServerAddress          = &serverAddress
)

func NewAPIFromStore(args NewAPIConnectionParams, open api.OpenFunc) (api.Connection, error) {
	return newAPIFromStore(args, open)
}
