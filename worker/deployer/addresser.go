// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"launchpad.net/juju-core/state/api/params"
)

// Addresser implementations provide the capability to lookup a list
// of server addresses.
type Addresser interface {
	StateAddresses() ([]string, error)
	APIAddresses() ([]string, error)
	ServerAddresses() (params.ServerAddressesResults, error)
}
