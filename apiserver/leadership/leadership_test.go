// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

/*
Test that the service is translating incoming parameters to the
manager layer correctly, and also translates the results back into
network parameters.
*/

import (
	"testing"
	"time"

	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

func Test(t *testing.T) { gc.TestingT(t) }

var (
	_ = gc.Suite(&leadershipSuite{})
	// Ensure the LeadershipService conforms to the interface at compile-time.
	_ LeadershipService = (*leadershipService)(nil)
)

type leadershipSuite struct{}

const (
	StubServiceNm = "stub-service"
	StubUnitNm    = "stub-unit/0"
)

type stubLeadershipManager struct {
	ClaimLeadershipFn              func(sid, uid string) (time.Duration, error)
	ReleaseLeadershipFn            func(sid, uid string) error
	BlockUntilLeadershipReleasedFn func(serviceId string) error
}

func (m *stubLeadershipManager) ClaimLeadership(sid, uid string) (time.Duration, error) {
	if m.ClaimLeadershipFn != nil {
		return m.ClaimLeadershipFn(sid, uid)
	}
	return 0, nil
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
func (m *stubAuthorizer) GetAuthTag() names.Tag    { return names.NewServiceTag(StubServiceNm) }

func (s *leadershipSuite) TestClaimLeadershipTranslation(c *gc.C) {
	var ldrMgr stubLeadershipManager
	ldrMgr.ClaimLeadershipFn = func(sid, uid string) (time.Duration, error) {
		c.Check(sid, gc.Equals, StubServiceNm)
		c.Check(uid, gc.Equals, StubUnitNm)
		return 0, nil
	}

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr, authorizer: &stubAuthorizer{}}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			params.ClaimLeadershipParams{
				ServiceTag: names.NewServiceTag(StubServiceNm).String(),
				UnitTag:    names.NewUnitTag(StubUnitNm).String(),
			},
		},
	})

	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
}

func (s *leadershipSuite) TestReleaseLeadershipTranslation(c *gc.C) {

	var ldrMgr stubLeadershipManager
	ldrMgr.ReleaseLeadershipFn = func(sid, uid string) error {
		c.Check(sid, gc.Equals, StubServiceNm)
		c.Check(uid, gc.Equals, StubUnitNm)
		return nil
	}

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr, authorizer: &stubAuthorizer{}}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			params.ClaimLeadershipParams{
				ServiceTag: names.NewServiceTag(StubServiceNm).String(),
				UnitTag:    names.NewUnitTag(StubUnitNm).String(),
			},
		},
	})

	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
}

func (s *leadershipSuite) TestBlockUntilLeadershipReleasedTranslation(c *gc.C) {

	var ldrMgr stubLeadershipManager
	ldrMgr.BlockUntilLeadershipReleasedFn = func(sid string) error {
		c.Check(sid, gc.Equals, StubServiceNm)
		return nil
	}

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr, authorizer: &stubAuthorizer{}}
	result, err := ldrSvc.BlockUntilLeadershipReleased(names.NewServiceTag(StubServiceNm))

	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *leadershipSuite) TestClaimLeadershipFailOnAuthorizerErrors(c *gc.C) {
	authorizer := &stubAuthorizer{
		AuthUnitAgentFn: func() bool { return false },
	}

	ldrSvc := &leadershipService{LeadershipManager: nil, authorizer: authorizer}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			params.ClaimLeadershipParams{
				ServiceTag: names.NewServiceTag(StubServiceNm).String(),
				UnitTag:    names.NewUnitTag(StubUnitNm).String(),
			},
		},
	})

	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.NotNil)
	c.Check(results.Results[0].Error, gc.ErrorMatches, common.ErrPerm.Error())
}

func (s *leadershipSuite) TestReleaseLeadershipFailOnAuthorizerErrors(c *gc.C) {
	authorizer := &stubAuthorizer{
		AuthUnitAgentFn: func() bool { return false },
	}

	ldrSvc := &leadershipService{LeadershipManager: nil, authorizer: authorizer}
	results, err := ldrSvc.ClaimLeadership(params.ClaimLeadershipBulkParams{
		Params: []params.ClaimLeadershipParams{
			params.ClaimLeadershipParams{
				ServiceTag: names.NewServiceTag(StubServiceNm).String(),
				UnitTag:    names.NewUnitTag(StubUnitNm).String(),
			},
		},
	})

	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.NotNil)
	c.Check(results.Results[0].Error, gc.ErrorMatches, common.ErrPerm.Error())
}

func (s *leadershipSuite) TestBlockUntilLeadershipReleasedErrors(c *gc.C) {
	authorizer := &stubAuthorizer{
		AuthUnitAgentFn: func() bool { return false },
	}

	ldrSvc := &leadershipService{LeadershipManager: nil, authorizer: authorizer}
	result, err := ldrSvc.BlockUntilLeadershipReleased(names.NewServiceTag(StubServiceNm))

	// Overall function call should succeed, but operations should
	// fail with a permissions issue.
	c.Assert(err, gc.IsNil)
	c.Check(result.Error, gc.ErrorMatches, common.ErrPerm.Error())
}
