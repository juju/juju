// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
)

// LeadershipService implements a variant of leadership.Claimer for consumption
// over the API.
type LeadershipService interface {

	// ClaimLeadership makes a leadership claim with the given parameters.
	ClaimLeadership(params params.ClaimLeadershipBulkParams) (params.ClaimLeadershipBulkResults, error)

	// BlockUntilLeadershipReleased blocks the caller until leadership is
	// released for the given service.
	BlockUntilLeadershipReleased(ApplicationTag names.ApplicationTag) (params.ErrorResult, error)
}
