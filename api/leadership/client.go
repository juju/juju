// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package leadership implements the client to the analog leadership
service.
*/
package leadership

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.leadership")

type facadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

type client struct {
	base.ClientFacade
	facadeCaller
}

// NewClient returns a new LeadershipClient instance.
func NewClient(facade base.ClientFacade, caller facadeCaller) LeadershipClient {
	return &client{facade, caller}
}

// ClaimLeadership implements LeadershipManager.
func (c *client) ClaimLeadership(serviceId, unitId string) (time.Duration, error) {

	params := c.prepareClaimLeadership(serviceId, unitId)
	results, err := c.bulkClaimLeadership(params.Params...)
	if err != nil {
		return 0, err
	}

	// We should have our 1 result. If not, we rightfully panic.
	result := results.Results[0]
	return time.Duration(result.ClaimDurationInSec) * time.Second, result.Error
}

// ReleaseLeadership implements LeadershipManager.
func (c *client) ReleaseLeadership(serviceId, unitId string) error {
	params := c.prepareReleaseLeadership(serviceId, unitId)
	results, err := c.bulkReleaseLeadership(params.Params...)
	if err != nil {
		return err
	}

	// We should have our 1 result. If not, we rightfully panic.
	return results.Errors[0]
}

// BlockUntilLeadershipReleased implements LeadershipManager.
func (c *client) BlockUntilLeadershipReleased(serviceId string) error {
	var result *params.Error
	err := c.FacadeCall("BlockUntilLeadershipReleased", names.NewServiceTag(serviceId), result)
	if err != nil {
		return errors.Annotate(err, "error blocking on leadership release")
	}
	return result
}

//
// Prepare functions for building bulk-calls.
//

// prepareClaimLeadership creates a single set of params in
// preperation for making a bulk call.
func (c *client) prepareClaimLeadership(serviceId, unitId string) params.ClaimLeadershipBulkParams {
	return params.ClaimLeadershipBulkParams{
		[]params.ClaimLeadershipParams{
			params.ClaimLeadershipParams{
				names.NewServiceTag(serviceId),
				names.NewUnitTag(unitId),
			},
		},
	}
}

// prepareReleaseLeadership creates a single set of params in
// preperation for making a bulk call.
func (c *client) prepareReleaseLeadership(serviceId, unitId string) params.ReleaseLeadershipBulkParams {
	return params.ReleaseLeadershipBulkParams{
		[]params.ReleaseLeadershipParams{
			params.ReleaseLeadershipParams{
				names.NewServiceTag(serviceId),
				names.NewUnitTag(unitId),
			},
		},
	}
}

//
// Bulk calls.
//

func (c *client) bulkClaimLeadership(args ...params.ClaimLeadershipParams) (*params.ClaimLeadershipBulkResults, error) {
	// Don't make the jump over the network if we don't have to.
	if len(args) <= 0 {
		return &params.ClaimLeadershipBulkResults{}, nil
	}

	// Translate & collect wire-format args.
	var wireParams []params.ClaimLeadershipParams
	for _, arg := range args {
		wireParams = append(wireParams, params.ClaimLeadershipParams(arg))
	}

	bulkParams := params.ClaimLeadershipBulkParams{wireParams}
	var results params.ClaimLeadershipBulkResults
	if err := c.FacadeCall("ClaimLeadership", bulkParams, &results); err != nil {
		return nil, errors.Annotate(err, "error making a leadership claim")
	}
	return &results, nil
}

func (c *client) bulkReleaseLeadership(args ...params.ReleaseLeadershipParams) (*params.ReleaseLeadershipBulkResults, error) {
	// Don't make the jump over the network if we don't have to.
	if len(args) <= 0 {
		return &params.ReleaseLeadershipBulkResults{}, nil
	}

	// Translate & collect wire-format args.
	var wireParams []params.ReleaseLeadershipParams
	for _, arg := range args {
		wireParams = append(wireParams, params.ReleaseLeadershipParams(arg))
	}

	bulkParams := params.ReleaseLeadershipBulkParams{wireParams}
	var results params.ReleaseLeadershipBulkResults
	if err := c.FacadeCall("ReleaseLeadership", bulkParams, &results); err != nil {
		return nil, errors.Annotate(err, "error attempting to release leadership")
	}
	return &results, nil
}
