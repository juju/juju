// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/leadership"
)

// LeadershipClaimDeniedErr is the error which will be returned when a
// leadership claim has been denied.
var LeadershipClaimDeniedErr = errors.New("leadership claim denied")

// LeadershipClient represents a client to the leadership service.
type LeadershipClient interface {
	base.ClientFacade
	leadership.LeadershipManager
}
