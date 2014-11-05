package leadership

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	leadershipsvc "github.com/juju/juju/apiserver/leadership"
	"github.com/juju/juju/leadership"
)

var LeadershipClaimDeniedErr = errors.New("the leadership claim has been denied.")

type LeadershipClient interface {
	base.ClientFacade
	leadership.LeadershipManager
	PrepareClaimLeadership(serviceId, unitId string) leadershipsvc.ClaimLeadershipParams
	PrepareReleaseLeadership(serviceId, unitId string) leadershipsvc.ReleaseLeadershipParams
	BulkClaimLeadership(...leadershipsvc.ClaimLeadershipParams) (*leadershipsvc.ClaimLeadershipBulkResults, error)
	BulkReleaseLeadership(...leadershipsvc.ReleaseLeadershipParams) (*leadershipsvc.ReleaseLeadershipBulkResults, error)
}
