// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllernode

// APIAddress represents one of the API addresses, accessible for clients
// and/or agents.
type APIAddress struct {
	// Address is the address of the API represented as "host:port" string.
	Address string

	// IsAgent indicates whether the address is available for agents.
	IsAgent bool
}
