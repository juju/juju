package leadership

/*
Test that the service is translating incoming parameters to the
manager layer correctly, and also translates the results back into
network parameters.
*/

import (
	"github.com/juju/names"
	gc "gopkg.in/check.v1"
	"testing"
	"time"
)

func Test(t *testing.T) { gc.TestingT(t) }

var _ = gc.Suite(&leadershipSuite{})

type leadershipSuite struct{}

const (
	StubServiceNm = "stub-service"
	StubUnitNm    = "stub-unit/0"
)

type Foo stubLeadershipManager

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

func (s *leadershipSuite) TestClaimLeadershipTranslation(c *gc.C) {
	var ldrMgr stubLeadershipManager
	ldrMgr.ClaimLeadershipFn = func(sid, uid string) (time.Duration, error) {
		c.Check(sid, gc.Equals, StubServiceNm)
		c.Check(uid, gc.Equals, StubUnitNm)
		return 0, nil
	}

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr}
	results, err := ldrSvc.ClaimLeadership(ClaimLeadershipBulkParams{
		Params: []ClaimLeadershipParams{
			ClaimLeadershipParams{
				ServiceTag: names.NewServiceTag(StubServiceNm),
				UnitTag:    names.NewUnitTag(StubUnitNm),
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

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr}
	results, err := ldrSvc.ClaimLeadership(ClaimLeadershipBulkParams{
		Params: []ClaimLeadershipParams{
			ClaimLeadershipParams{
				ServiceTag: names.NewServiceTag(StubServiceNm),
				UnitTag:    names.NewUnitTag(StubUnitNm),
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

	ldrSvc := &leadershipService{LeadershipManager: &ldrMgr}
	err := ldrSvc.BlockUntilLeadershipReleased(names.NewServiceTag(StubServiceNm))

	c.Assert(err, gc.IsNil)
}
