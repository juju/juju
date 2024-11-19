// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package connector

import (
	"github.com/juju/juju/api"
)

// A Connector is able to provide a Connection.  This connection can be used to
// make API calls via the various packages in github.com/juju/juju/api.
type Connector interface {
	Connect(...api.DialOption) (api.Connection, error)
}
