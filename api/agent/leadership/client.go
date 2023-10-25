// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

type client struct {
	base.FacadeCaller
}

// NewClient returns a new leadership.Claimer backed by the supplied api caller.
func NewClient(caller base.APICaller, options ...Option) leadership.Claimer {
	return &client{base.NewFacadeCaller(caller, "LeadershipService", options...)}
}

// ClaimLeadership is part of the leadership.Claimer interface.
func (c *client) ClaimLeadership(appId, unitId string, duration time.Duration) error {
	results, err := c.bulkClaimLeadership(c.prepareClaimLeadership(appId, unitId, duration))
	if err != nil {
		return err
	}

	// TODO(fwereade): this is not a rightful panic; we don't know who'll be using
	// this client, and/or whether or not we're running critical code in the same
	// process.
	if err := results.Results[0].Error; err != nil {
		if params.IsCodeLeadershipClaimDenied(err) {
			return leadership.ErrClaimDenied
		}
		return err
	}
	return nil
}

// BlockUntilLeadershipReleased is part of the leadership.Claimer interface.
func (c *client) BlockUntilLeadershipReleased(appId string, cancel <-chan struct{}) error {
	const friendlyErrMsg = "error blocking on leadership release"
	var result params.ErrorResult
	// TODO(axw) make it possible to plumb a context.Context
	// through the API/RPC client, so we can cancel or abandon
	// requests.
	err := c.FacadeCall(context.TODO(), "BlockUntilLeadershipReleased", names.NewApplicationTag(appId), &result)
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
// preparation for making a bulk call.
func (c *client) prepareClaimLeadership(appId, unitId string, duration time.Duration) params.ClaimLeadershipParams {
	return params.ClaimLeadershipParams{
		names.NewApplicationTag(appId).String(),
		names.NewUnitTag(unitId).String(),
		duration.Seconds(),
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
	if err := c.FacadeCall(context.TODO(), "ClaimLeadership", bulkParams, &results); err != nil {
		return nil, errors.Annotate(err, "error making a leadership claim")
	}
	return &results, nil
}
