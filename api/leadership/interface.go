// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/leadership"
)

// LeadershipClient represents a client to the leadership service.
type LeadershipClient interface {
	base.ClientFacade
	leadership.LeadershipManager
}
