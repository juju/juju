// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

// DetailsTopic is the topic name for the published message when the details
// of the api servers change. This message is normally published by the
// peergrouper when the set of API servers changes.
const DetailsTopic = "apiserver.details"

// APIServer contains the machine id and addresses of a single API server machine.
type APIServer struct {
	// ID contains the Juju machine ID for the apiserver.
	ID string `yaml:"id"`

	// Addresses contains all of the usable addresses of the
	// apiserver machine.
	Addresses []string `yaml:"addresses"`

	// InternalAddress, if non-empty, is the address that
	// other API servers should use to connect to this API
	// server, in the form addr:port.
	//
	// This may be empty if the API server is not fully
	// initialised.
	InternalAddress string `yaml:"internal-address,omitempty"`
}

// Details contains the ids and addresses of all the current API server
// machines.
type Details struct {
	// Servers is a map of machine ID to the details for that server.
	Servers   map[string]APIServer `yaml:"servers"`
	LocalOnly bool                 `yaml:"local-only"`
}
