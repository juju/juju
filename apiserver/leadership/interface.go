package leadership

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

type ClaimLeadershipBulkParams struct {
	Params []ClaimLeadershipParams
}

type ClaimLeadershipParams struct {
	ServiceTag names.ServiceTag
	UnitTag    names.UnitTag
}

type ClaimLeadershipBulkResults struct {
	Results []ClaimLeadershipResults
}

type ClaimLeadershipResults struct {
	ServiceTag         names.ServiceTag
	ClaimDurationInSec float64
	Error              *params.Error
}

type ReleaseLeadershipBulkParams struct {
	Params []ReleaseLeadershipParams
}

type ReleaseLeadershipParams struct {
	ServiceTag names.ServiceTag
	UnitTag    names.UnitTag
}

type ReleaseLeadershipBulkResults struct {
	Errors []*params.Error
}

type LeadershipService interface {
	ClaimLeadership(params ClaimLeadershipBulkParams) (ClaimLeadershipBulkResults, error)
	ReleaseLeadership(params ReleaseLeadershipBulkParams) (ReleaseLeadershipBulkResults, error)
	BlockUntilLeadershipReleased(serviceTag names.ServiceTag) (err error)
}
