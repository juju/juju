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
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/leadership"
	"github.com/juju/juju/apiserver/params"
	coreleadership "github.com/juju/juju/core/leadership"
)

type leadershipSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&leadershipSuite{})

const (
	StubAppNm  = "stub-application"
	StubUnitNm = "stub-application/0"
)

type stubClaimer struct {
	ClaimLeadershipFn              func(sid, uid string, duration time.Duration) error
	BlockUntilLeadershipReleasedFn func(serviceId string, cancel <-chan struct{}) error
}

func (m *stubClaimer) ClaimLeadership(sid, uid string, duration time.Duration) error {
	if m.ClaimLeadershipFn != nil {
		return m.ClaimLeadershipFn(sid, uid, duration)
	}
	return nil
}

func (m *stubClaimer) BlockUntilLeadershipReleased(serviceId string, cancel <-chan struct{}) error {
	if m.BlockUntilLeadershipReleasedFn != nil {
		return m.BlockUntilLeadershipReleasedFn(serviceId, cancel)
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

func checkDurationEquals(c *gc.C, actual, expect time.Duration) {
	delta := actual - expect
	if delta < 0 {
		delta = -delta
	}
	c.Check(delta, jc.LessThan, time.Microsecond)
}

func newLeadershipService(
	c *gc.C, claimer coreleadership.Claimer, authorizer facade.Authorizer,
) leadership.LeadershipService {
	if authorizer == nil {
		authorizer = stubAuthorizer{tag: names.NewUnitTag(StubUnitNm)}
	}
	result, err := leadership.NewLeadershipService(claimer, authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return result
}

func (s *leadershipSuite) TestClaimLeadershipTranslation(c *gc.C) {
	claimer := &stubClaimer{
		ClaimLeadershipFn: func(sid, uid string, duration time.Duration) error {
			c.Check(sid, gc.Equals, StubAppNm)
			c.Check(uid, gc.Equals, StubUnitNm)
			expectDuration := time.Duration(299.9 * float64(time.Second))
			checkDurationEquals(c, duration, expectDuration)
			return nil
		},
	}

	ldrSvc := newLeadershipService(c, claimer, nil)
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 299.9,
			},
		},
	})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.IsNil)
}

func (s *leadershipSuite) TestClaimLeadershipApplicationAgent(c *gc.C) {
	claimer := &stubClaimer{
		ClaimLeadershipFn: func(sid, uid string, duration time.Duration) error {
			c.Check(sid, gc.Equals, StubAppNm)
			c.Check(uid, gc.Equals, StubUnitNm)
			expectDuration := time.Duration(299.9 * float64(time.Second))
			checkDurationEquals(c, duration, expectDuration)
			return nil
		},
	}

	authorizer := &stubAuthorizer{
		tag: names.NewApplicationTag(StubAppNm),
	}
	ldrSvc := newLeadershipService(c, claimer, authorizer)
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 299.9,
			},
		},
	})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.IsNil)
}

func (s *leadershipSuite) TestClaimLeadershipDeniedError(c *gc.C) {
	claimer := &stubClaimer{
		ClaimLeadershipFn: func(sid, uid string, duration time.Duration) error {
			c.Check(sid, gc.Equals, StubAppNm)
			c.Check(uid, gc.Equals, StubUnitNm)
			expectDuration := time.Duration(5.001 * float64(time.Second))
			checkDurationEquals(c, duration, expectDuration)
			return errors.Annotatef(coreleadership.ErrClaimDenied, "obfuscated")
		},
	}

	ldrSvc := newLeadershipService(c, claimer, nil)
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 5.001,
			},
		},
	})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, jc.Satisfies, params.IsCodeLeadershipClaimDenied)
}

func (s *leadershipSuite) TestClaimLeadershipBadService(c *gc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)

	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  "application-bad/0",
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 123.45,
			},
		},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestClaimLeadershipBadUnit(c *gc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)

	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         "unit-bad",
				DurationSeconds: 123.45,
			},
		},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestClaimLeadershipDurationTooShort(c *gc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)

	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 4.99,
			},
		},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.ErrorMatches, "invalid duration")
}

func (s *leadershipSuite) TestClaimLeadershipDurationTooLong(c *gc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)

	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 300.1,
			},
		},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.ErrorMatches, "invalid duration")
}

func (s *leadershipSuite) TestBlockUntilLeadershipReleasedTranslation(c *gc.C) {
	claimer := &stubClaimer{
		BlockUntilLeadershipReleasedFn: func(sid string, cancel <-chan struct{}) error {
			c.Check(sid, gc.Equals, StubAppNm)
			return nil
		},
	}

	ldrSvc := newLeadershipService(c, claimer, nil)
	result, err := ldrSvc.BlockUntilLeadershipReleased(
		context.Background(),
		names.NewApplicationTag(StubAppNm),
	)

	c.Check(err, jc.ErrorIsNil)
	c.Check(result.Error, gc.IsNil)
}

func (s *leadershipSuite) TestBlockUntilLeadershipReleasedContext(c *gc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	claimer := &stubClaimer{
		BlockUntilLeadershipReleasedFn: func(sid string, cancel <-chan struct{}) error {
			c.Check(sid, gc.Equals, StubAppNm)
			c.Check(cancel, gc.Equals, ctx.Done())
			return coreleadership.ErrBlockCancelled
		},
	}

	ldrSvc := newLeadershipService(c, claimer, nil)
	result, err := ldrSvc.BlockUntilLeadershipReleased(
		ctx,
		names.NewApplicationTag(StubAppNm),
	)

	c.Check(err, jc.ErrorIsNil)
	c.Check(result.Error, gc.ErrorMatches, "waiting for leadership cancelled by client")
}

func (s *leadershipSuite) TestClaimLeadershipFailBadUnit(c *gc.C) {
	authorizer := &stubAuthorizer{
		tag: names.NewUnitTag("lol-different/123"),
	}

	ldrSvc := newLeadershipService(c, nil, authorizer)
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag(StubAppNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 123.45,
			},
		},
	})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Check(results.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestClaimLeadershipFailBadService(c *gc.C) {
	ldrSvc := newLeadershipService(c, nil, nil)
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ApplicationTag:  names.NewApplicationTag("lol-different").String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 123.45,
			},
		},
	})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Check(results.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestCreateUnauthorized(c *gc.C) {
	authorizer := &stubAuthorizer{
		tag: names.NewMachineTag("123"),
	}

	ldrSvc, err := leadership.NewLeadershipService(nil, authorizer)
	c.Check(ldrSvc, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "permission denied")
	c.Check(err, jc.Satisfies, errors.IsUnauthorized)
}
