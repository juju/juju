// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package leadership implements the client to the analog leadership
service.
*/
package leadership

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

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

	results, err := c.bulkClaimLeadership(c.prepareClaimLeadership(serviceId, unitId))
	if err != nil {
		return 0, err
	}

	// We should have our 1 result. If not, we rightfully panic.
	result := results.Results[0]
	return time.Duration(result.ClaimDurationInSec) * time.Second, result.Error
}

// ReleaseLeadership implements LeadershipManager.
func (c *client) ReleaseLeadership(serviceId, unitId string) error {
	results, err := c.bulkReleaseLeadership(c.prepareReleaseLeadership(serviceId, unitId))
	if err != nil {
		return err
	}

	// We should have our 1 result. If not, we rightfully panic.
	return results.Results[0].Error
}

// BlockUntilLeadershipReleased implements LeadershipManager.
func (c *client) BlockUntilLeadershipReleased(serviceId string) error {
	const friendlyErrMsg = "error blocking on leadership release"
	var result params.ErrorResult
	err := c.FacadeCall("BlockUntilLeadershipReleased", names.NewServiceTag(serviceId), &result)
	if err != nil {
		return errors.Annotate(err, friendlyErrMsg)
	} else if result.Error != nil {
		return errors.Annotate(result.Error, friendlyErrMsg)
	}
	return nil
}

//
// Prepare functions for building bulk-calls.
//

// prepareClaimLeadership creates a single set of params in
// preperation for making a bulk call.
func (c *client) prepareClaimLeadership(serviceId, unitId string) params.ClaimLeadershipParams {
	return params.ClaimLeadershipParams{
		names.NewServiceTag(serviceId).String(),
		names.NewUnitTag(unitId).String(),
	}
}

// prepareReleaseLeadership creates a single set of params in
// preperation for making a bulk call.
func (c *client) prepareReleaseLeadership(serviceId, unitId string) params.ReleaseLeadershipParams {
	return params.ReleaseLeadershipParams{
		names.NewServiceTag(serviceId).String(),
		names.NewUnitTag(unitId).String(),
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

	bulkParams := params.ClaimLeadershipBulkParams{args}
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

	bulkParams := params.ReleaseLeadershipBulkParams{args}
	var results params.ReleaseLeadershipBulkResults
	if err := c.FacadeCall("ReleaseLeadership", bulkParams, &results); err != nil {
		return nil, errors.Annotate(err, "cannot release leadership")
	}

	return &results, nil
}
