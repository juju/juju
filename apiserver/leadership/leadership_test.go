// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

/*
Test that the service is translating incoming parameters to the
manager layer correctly, and also translates the results back into
network parameters.
*/

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/leadership"
)

type leadershipSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&leadershipSuite{})

const (
	StubServiceNm = "stub-service"
	StubUnitNm    = "stub-unit/0"
)

type stubLeadershipManager struct {
	ClaimLeadershipFn              func(sid, uid string, duration time.Duration) error
	ReleaseLeadershipFn            func(sid, uid string) error
	BlockUntilLeadershipReleasedFn func(serviceId string) error
}

func (m *stubLeadershipManager) ClaimLeadership(sid, uid string, duration time.Duration) error {
	if m.ClaimLeadershipFn != nil {
		return m.ClaimLeadershipFn(sid, uid, duration)
	}
	return nil
}

func (m *stubLeadershipManager) ReleaseLeadership(sid, uid string) error {
	if m.ReleaseLeadershipFn != nil {
		return m.ReleaseLeadershipFn(sid, uid)
	}
	return nil
}

func (m *stubLeadershipManager) BlockUntilLeadershipReleased(serviceId string) error {
	if m.BlockUntilLeadershipReleasedFn != nil {
		return m.BlockUntilLeadershipReleasedFn(serviceId)
	}
	return nil
}

type stubAuthorizer struct {
	AuthOwnerFn     func(names.Tag) bool
	AuthUnitAgentFn func() bool
}

func (m *stubAuthorizer) AuthMachineAgent() bool { return true }
func (m *stubAuthorizer) AuthUnitAgent() bool {
	if m.AuthUnitAgentFn != nil {
		return m.AuthUnitAgentFn()
	}
	return true
}
func (m *stubAuthorizer) AuthOwner(tag names.Tag) bool {
	if m.AuthOwnerFn != nil {
		return m.AuthOwnerFn(tag)
	}
	return true
}
func (m *stubAuthorizer) AuthEnvironManager() bool { return true }
func (m *stubAuthorizer) AuthClient() bool         { return true }
func (m *stubAuthorizer) GetAuthTag() names.Tag    { return names.NewServiceTag(StubUnitNm) }

func checkDurationEquals(c *gc.C, actual, expect time.Duration) {
	delta := actual - expect
	if delta < 0 {
		delta = -delta
	}
	c.Check(delta, jc.LessThan, time.Microsecond)
}

func (s *leadershipSuite) TestClaimLeadershipTranslation(c *gc.C) {
	var ldrMgr stubLeadershipManager
	ldrMgr.ClaimLeadershipFn = func(sid, uid string, duration time.Duration) error {
		c.Check(sid, gc.Equals, StubServiceNm)
		c.Check(uid, gc.Equals, StubUnitNm)
		expectDuration := time.Duration(299.9 * float64(time.Second))
		checkDurationEquals(c, duration, expectDuration)
		return nil
	}

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr, authorizer: &stubAuthorizer{}}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ServiceTag:      names.NewServiceTag(StubServiceNm).String(),
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
	var ldrMgr stubLeadershipManager
	ldrMgr.ClaimLeadershipFn = func(sid, uid string, duration time.Duration) error {
		c.Check(sid, gc.Equals, StubServiceNm)
		c.Check(uid, gc.Equals, StubUnitNm)
		expectDuration := time.Duration(5.001 * float64(time.Second))
		checkDurationEquals(c, duration, expectDuration)
		return errors.Annotatef(leadership.ErrClaimDenied, "obfuscated")
	}

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr, authorizer: &stubAuthorizer{}}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ServiceTag:      names.NewServiceTag(StubServiceNm).String(),
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
	ldrSvc := &leadershipService{authorizer: &stubAuthorizer{}}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ServiceTag:      "service-bad/0",
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
	ldrSvc := &leadershipService{authorizer: &stubAuthorizer{}}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ServiceTag:      names.NewServiceTag(StubServiceNm).String(),
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
	ldrSvc := &leadershipService{authorizer: &stubAuthorizer{}}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ServiceTag:      names.NewServiceTag(StubServiceNm).String(),
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
	ldrSvc := &leadershipService{authorizer: &stubAuthorizer{}}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ServiceTag:      names.NewServiceTag(StubServiceNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 300.1,
			},
		},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.ErrorMatches, "invalid duration")
}

func (s *leadershipSuite) TestReleaseLeadershipTranslation(c *gc.C) {

	var ldrMgr stubLeadershipManager
	ldrMgr.ReleaseLeadershipFn = func(sid, uid string) error {
		c.Check(sid, gc.Equals, StubServiceNm)
		c.Check(uid, gc.Equals, StubUnitNm)
		return nil
	}

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr, authorizer: &stubAuthorizer{}}
	results, err := ldrSvc.ReleaseLeadership(params.ReleaseLeadershipBulkParams{
		Params: []params.ReleaseLeadershipParams{
			{
				ServiceTag: names.NewServiceTag(StubServiceNm).String(),
				UnitTag:    names.NewUnitTag(StubUnitNm).String(),
			},
		},
	})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, gc.IsNil)
}

func (s *leadershipSuite) TestBlockUntilLeadershipReleasedTranslation(c *gc.C) {

	var ldrMgr stubLeadershipManager
	ldrMgr.BlockUntilLeadershipReleasedFn = func(sid string) error {
		c.Check(sid, gc.Equals, StubServiceNm)
		return nil
	}

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr, authorizer: &stubAuthorizer{}}
	result, err := ldrSvc.BlockUntilLeadershipReleased(names.NewServiceTag(StubServiceNm))

	c.Check(err, jc.ErrorIsNil)
	c.Check(result.Error, gc.IsNil)
}

func (s *leadershipSuite) TestClaimLeadershipFailOnAuthorizerErrors(c *gc.C) {
	authorizer := &stubAuthorizer{
		AuthUnitAgentFn: func() bool { return false },
	}

	ldrSvc := &leadershipService{LeadershipManager: nil, authorizer: authorizer}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			{
				ServiceTag:      names.NewServiceTag(StubServiceNm).String(),
				UnitTag:         names.NewUnitTag(StubUnitNm).String(),
				DurationSeconds: 123.45,
			},
		},
	})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestReleaseLeadershipFailOnAuthorizerErrors(c *gc.C) {
	authorizer := &stubAuthorizer{
		AuthUnitAgentFn: func() bool { return false },
	}

	ldrSvc := &leadershipService{LeadershipManager: nil, authorizer: authorizer}
	results, err := ldrSvc.ReleaseLeadership(params.ReleaseLeadershipBulkParams{
		Params: []params.ReleaseLeadershipParams{
			{
				ServiceTag: names.NewServiceTag(StubServiceNm).String(),
				UnitTag:    names.NewUnitTag(StubUnitNm).String(),
			},
		},
	})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *leadershipSuite) TestBlockUntilLeadershipReleasedErrors(c *gc.C) {
	authorizer := &stubAuthorizer{
		AuthUnitAgentFn: func() bool { return false },
	}

	ldrSvc := &leadershipService{LeadershipManager: nil, authorizer: authorizer}
	result, err := ldrSvc.BlockUntilLeadershipReleased(names.NewServiceTag(StubServiceNm))

	// Overall function call should succeed, but operations should
	// fail with a permissions issue.
	c.Check(err, jc.ErrorIsNil)
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}
