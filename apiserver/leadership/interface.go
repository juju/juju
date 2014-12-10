package leadership

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

type LeadershipService interface {
	// ClaimLeadership makes a leadership claim with the given parameters.
	ClaimLeadership(params params.ClaimLeadershipBulkParams) (params.ClaimLeadershipBulkResults, error)
	// ReleaseLeadership makes a call to release leadership for all the
	// parameters passed in.
	ReleaseLeadership(params params.ReleaseLeadershipBulkParams) (params.ReleaseLeadershipBulkResults, error)
	// BlockUntilLeadershipReleased blocks the caller until leadership is
	// released for the given service.
	BlockUntilLeadershipReleased(serviceTag names.ServiceTag) (err error)
}
