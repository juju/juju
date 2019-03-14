// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

// Address describes a network address.
type Address struct {
	Value           string
	Type            string
	Scope           string
	SpaceName       string
	SpaceProviderId string
}
