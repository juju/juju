package leadership

import (
	"time"

	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/leadership"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"
)

var (
	logger                  = loggo.GetLogger("juju.api.leadership")
	_      LeadershipClient = (*client)(nil)
)

type facadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

type client struct {
	base.ClientFacade
	facadeCaller
}

func NewClient(facade base.ClientFacade, caller facadeCaller) LeadershipClient {
	return &client{facade, caller}
}

func (c *client) ClaimLeadership(serviceId, unitId string) (time.Duration, error) {

	params := c.PrepareClaimLeadership(serviceId, unitId)
	results, err := c.BulkClaimLeadership(params)
	if err != nil {
		return 0, err
	}

	// We should have our 1 result. If not, we rightfully panic.
	result := results.Results[0]
	return time.Duration(result.ClaimDurationInSec) * time.Second, result.Error
}

func (c *client) ReleaseLeadership(serviceId, unitId string) error {
	params := c.PrepareReleaseLeadership(serviceId, unitId)
	results, err := c.BulkReleaseLeadership(params)
	if err != nil {
		return err
	}

	// We should have our 1 result. If not, we rightfully panic.
	return results.Errors[0]
}

func (c *client) BlockUntilLeadershipReleased(serviceId string) error {
	var result *params.Error
	err := c.FacadeCall("BlockUntilLeadershipReleased", names.NewServiceTag(serviceId), result)
	if err != nil {
		return err
	}
	return result
}

//
// Prepare functions for building bulk-calls.
//

func (c *client) PrepareClaimLeadership(serviceId, unitId string) leadership.ClaimLeadershipParams {
	return leadership.ClaimLeadershipParams{
		names.NewServiceTag(serviceId),
		names.NewUnitTag(unitId),
	}
}

func (c *client) PrepareReleaseLeadership(serviceId, unitId string) leadership.ReleaseLeadershipParams {
	return leadership.ReleaseLeadershipParams{
		names.NewServiceTag(serviceId),
		names.NewUnitTag(unitId),
	}
}

//
// Bulk calls.
//

func (c *client) BulkClaimLeadership(params ...leadership.ClaimLeadershipParams) (*leadership.ClaimLeadershipBulkResults, error) {
	// Don't make the jump over the network if we don't have to.
	if len(params) <= 0 {
		return &leadership.ClaimLeadershipBulkResults{}, nil
	}

	bulkParams := leadership.ClaimLeadershipBulkParams{params}
	var results leadership.ClaimLeadershipBulkResults
	if err := c.FacadeCall("ClaimLeadership", bulkParams, &results); err != nil {
		return nil, err
	}
	return &results, nil
}

func (c *client) BulkReleaseLeadership(params ...leadership.ReleaseLeadershipParams) (*leadership.ReleaseLeadershipBulkResults, error) {
	// Don't make the jump over the network if we don't have to.
	if len(params) <= 0 {
		return &leadership.ReleaseLeadershipBulkResults{}, nil
	}

	bulkParams := leadership.ReleaseLeadershipBulkParams{params}
	var results leadership.ReleaseLeadershipBulkResults
	if err := c.FacadeCall("ReleaseLeadership", bulkParams, &results); err != nil {
		return nil, err
	}
	return &results, nil
}
