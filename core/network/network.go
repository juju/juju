// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import "github.com/juju/loggo"

var logger = loggo.GetLogger("juju.core.network")

// Id defines a provider-specific network ID.
type Id string

// String returns the underlying string representation of the Id.
// This method helps with formatting and type inference.
func (id Id) String() string {
	return string(id)
}
