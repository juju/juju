// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

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
	return &client{FacadeCaller: base.NewFacadeCaller(caller, "LeadershipService", options...)}
}

// ClaimLeadership is part of the leadership.Claimer interface.
func (c *client) ClaimLeadership(ctx context.Context, appId, unitId string, duration time.Duration) error {
	results, err := c.bulkClaimLeadership(ctx, c.prepareClaimLeadership(appId, unitId, duration))
	if err != nil {
		return err
	}

	if err := results.Results[0].Error; err != nil {
		if params.IsCodeLeadershipClaimDenied(err) {
			return leadership.ErrClaimDenied
		}
		return err
	}
	return nil
}

// BlockUntilLeadershipReleased is part of the leadership.Claimer interface.
func (c *client) BlockUntilLeadershipReleased(ctx context.Context, appId string) error {
	const friendlyErrMsg = "error blocking on leadership release"
	var result params.ErrorResult
	err := c.FacadeCall(ctx, "BlockUntilLeadershipReleased", names.NewApplicationTag(appId), &result)
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
		ApplicationTag:  names.NewApplicationTag(appId).String(),
		UnitTag:         names.NewUnitTag(unitId).String(),
		DurationSeconds: duration.Seconds(),
	}
}

//
// Bulk calls.
//

func (c *client) bulkClaimLeadership(ctx context.Context, args ...params.ClaimLeadershipParams) (*params.ClaimLeadershipBulkResults, error) {
	// Don't make the jump over the network if we don't have to.
	if len(args) <= 0 {
		return &params.ClaimLeadershipBulkResults{}, nil
	}

	bulkParams := params.ClaimLeadershipBulkParams{Params: args}
	var results params.ClaimLeadershipBulkResults
	if err := c.FacadeCall(ctx, "ClaimLeadership", bulkParams, &results); err != nil {
		return nil, errors.Annotate(err, "error making a leadership claim")
	}
	return &results, nil
}
