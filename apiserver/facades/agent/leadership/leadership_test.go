// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

// Test that the service is translating incoming parameters to the
// manager layer correctly, and also translates the results back into
// network parameters.

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/leadership"
	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type leadershipSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&leadershipSuite{})

const (
	StubAppNm  = "stub-application"
	StubUnitNm = "stub-application/0"
)

type stubClaimer struct {
	ClaimLeadershipFn              func(ctx context.Context, sid, uid string, duration time.Duration) error
	BlockUntilLeadershipReleasedFn func(ctx context.Context, serviceId string) error
}

func (m *stubClaimer) ClaimLeadership(ctx context.Context, sid, uid string, duration time.Duration) error {
	if m.ClaimLeadershipFn != nil {
		return m.ClaimLeadershipFn(ctx, sid, uid, duration)
	}
	return nil
}

func (m *stubClaimer) BlockUntilLeadershipReleased(ctx context.Context, serviceId string) error {
	if m.BlockUntilLeadershipReleasedFn != nil {
		return m.BlockUntilLeadershipReleasedFn(ctx, serviceId)
	}
	return nil
}

type stubAuthorizer struct {
	facade.Authorizer
	tag names.Tag
}

func (m stubAuthorizer) AuthUnitAgent() bool {
	_, ok := m.tag.(names.UnitTag)
	return ok
}

func (m stubAuthorizer) AuthApplicationAgent() bool {
	_, ok := m.tag.(names.ApplicationTag)
	return ok
}

func (m stubAuthorizer) AuthOwner(tag names.Tag) bool {
	return tag == m.tag
}

func (m stubAuthorizer) GetAuthTag() names.Tag {
	return m.tag
}

func checkDurationEquals(c *tc.C, actual, expect time.Duration) {
	delta := actual - expect
	if delta < 0 {
		delta = -delta
	}
	c.Check(delta, tc.LessThan, time.Microsecond)
}

func newLeadershipService(
	c *tc.C, claimer coreleadership.Claimer, authorizer facade.Authorizer,
) leadership.LeadershipService {
	if authorizer == nil {
		authorizer = stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	}
	result, err := leadership.NewLeadershipService(claimer, authorizer)
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *leadershipSuite) TestClaimLeadershipTranslation(c *tc.C) {
	claimer := &stubClaimer{
		ClaimLeadershipFn: func(ctx context.Context, sid, uid string, duration time.Duration) error {
			c.Check(sid, tc.Equals, StubAppNm)
			c.Check(uid, tc.Equals, StubUnitNm)
			expectDuration := time.Duration(299.9 * float64(time.Second))
			checkDurationEquals(c, duration, expectDuration)
			return nil
		},
	}

	ldrSvc := newLeadershipService(c, claimer, nil)
	results, err := ldrSvc.ClaimLeadership(context.Background(), params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 299.9,
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.IsNil)
}

func (s *leadershipSuite) TestClaimLeadershipApplicationAgent(c *tc.C) {
	claimer := &stubClaimer{
		ClaimLeadershipFn: func(ctx context.Context, sid, uid string, duration time.Duration) error {
			c.Check(sid, tc.Equals, StubAppNm)
			c.Check(uid, tc.Equals, StubUnitNm)
			expectDuration := time.Duration(299.9 * float64(time.Second))
			checkDurationEquals(c, duration, expectDuration)
			return nil
		},
	}

	authorizer := &stubAuthorizer{
		tag: names.NewApplicationTag(StubAppNm),
	}
	ldrSvc := newLeadershipService(c, claimer, authorizer)
	results, err := ldrSvc.ClaimLeadership(context.Background(), params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 299.9,
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.IsNil)
}

func (s *leadershipSuite) TestClaimLeadershipDeniedError(c *tc.C) {
	claimer := &stubClaimer{
		ClaimLeadershipFn: func(ctx context.Context, sid, uid string, duration time.Duration) error {
			c.Check(sid, tc.Equals, StubAppNm)
			c.Check(uid, tc.Equals, StubUnitNm)
			expectDuration := time.Duration(5.001 * float64(time.Second))
			checkDurationEquals(c, duration, expectDuration)
			return errors.Annotatef(coreleadership.ErrClaimDenied, "obfuscated")
		},
	}

	ldrSvc := newLeadershipService(c, claimer, nil)
	results, err := ldrSvc.ClaimLeadership(context.Background(), params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 5.001,
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.Satisfies, params.IsCodeLeadershipClaimDenied)
}

func (s *leadershipSuite) TestClaimLeadershipBadService(c *tc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)

	results, err := ldrSvc.ClaimLeadership(context.Background(), params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  "application-bad/0",
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 123.45,
			},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestClaimLeadershipBadUnit(c *tc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)

	results, err := ldrSvc.ClaimLeadership(context.Background(), params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         "unit-bad",
				DurationSeconds: 123.45,
			},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestClaimLeadershipDurationTooShort(c *tc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)

	results, err := ldrSvc.ClaimLeadership(context.Background(), params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 4.99,
			},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.ErrorMatches, "invalid duration")
}

func (s *leadershipSuite) TestClaimLeadershipDurationTooLong(c *tc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)

	results, err := ldrSvc.ClaimLeadership(context.Background(), params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 300.1,
			},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.ErrorMatches, "invalid duration")
}

func (s *leadershipSuite) TestBlockUntilLeadershipReleasedTranslation(c *tc.C) {
	claimer := &stubClaimer{
		BlockUntilLeadershipReleasedFn: func(ctx context.Context, sid string) error {
			c.Check(sid, tc.Equals, StubAppNm)
			return nil
		},
	}

	ldrSvc := newLeadershipService(c, claimer, nil)
	result, err := ldrSvc.BlockUntilLeadershipReleased(
		context.Background(),
		names.NewApplicationTag(StubAppNm),
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Error, tc.IsNil)
}

func (s *leadershipSuite) TestBlockUntilLeadershipReleasedContext(c *tc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	claimer := &stubClaimer{
		BlockUntilLeadershipReleasedFn: func(ctx context.Context, sid string) error {
			c.Check(sid, tc.Equals, StubAppNm)
			c.Check(ctx.Err(), tc.Equals, context.Canceled)
			return coreleadership.ErrBlockCancelled
		},
	}

	ldrSvc := newLeadershipService(c, claimer, nil)
	result, err := ldrSvc.BlockUntilLeadershipReleased(
		ctx,
		names.NewApplicationTag(StubAppNm),
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Error, tc.ErrorMatches, "waiting for leadership cancelled by client")
}

func (s *leadershipSuite) TestClaimLeadershipFailBadUnit(c *tc.C) {
	authorizer := &stubAuthorizer{
		tag: names.NewUnitTag("lol-different/123"),
	}

	ldrSvc := newLeadershipService(c, nil, authorizer)
	results, err := ldrSvc.ClaimLeadership(context.Background(), params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 123.45,
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.ErrorMatches, "permission denied")
	c.Check(results.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestClaimLeadershipFailBadService(c *tc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)
	results, err := ldrSvc.ClaimLeadership(context.Background(), params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag("lol-different").String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 123.45,
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.ErrorMatches, "permission denied")
	c.Check(results.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestCreateUnauthorized(c *tc.C) {
	authorizer := &stubAuthorizer{
		tag: names.NewMachineTag("123"),
	}

	ldrSvc, err := leadership.NewLeadershipService(nil, authorizer)
	c.Check(ldrSvc, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "permission denied")
	c.Check(err, tc.ErrorIs, errors.Unauthorized)
}
