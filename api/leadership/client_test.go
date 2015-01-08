package leadership

import (
	"fmt"
	"testing"
	"time"

	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

/*
Test that the client is translating incoming parameters to the
service layer correctly, and also translates the results back
correctly.
*/

func Test(t *testing.T) { gc.TestingT(t) }

var _ = gc.Suite(&clientSuite{})

type clientSuite struct{}

const (
	StubServiceNm = "stub-service"
	StubUnitNm    = "stub-unit/0"
)

type stubFacade struct {
	FacadeCallFn func(string, interface{}, interface{}) error
}

func (s *stubFacade) FacadeCall(request string, params, response interface{}) error {
	if s.FacadeCallFn != nil {
		return s.FacadeCallFn(request, params, response)
	}
	return nil
}

func (s *stubFacade) BestAPIVersion() int { return -1 }
func (s *stubFacade) Close() error        { return nil }

func (s *clientSuite) TestClaimLeadershipTranslation(c *gc.C) {

	const claimTime = 5 * time.Hour
	numStubCalls := 0

	stub := &stubFacade{
		FacadeCallFn: func(name string, parameters, response interface{}) error {
			numStubCalls++
			c.Check(name, gc.Equals, "ClaimLeadership")
			c.Assert(parameters, gc.Not(gc.IsNil))

			typedP, ok := parameters.(params.ClaimLeadershipBulkParams)
			c.Assert(ok, gc.Equals, true)

			typedR, ok := response.(*params.ClaimLeadershipBulkResults)
			c.Assert(ok, gc.Equals, true)
			typedR.Results = []params.ClaimLeadershipResults{params.ClaimLeadershipResults{
				ClaimDurationInSec: claimTime.Seconds(),
			}}

			c.Assert(typedP.Params, gc.HasLen, 1)
			c.Check(typedP.Params[0].ServiceTag.Id(), gc.Equals, StubServiceNm)
			c.Check(typedP.Params[0].UnitTag.Id(), gc.Equals, StubUnitNm)

			return nil
		},
	}

	client := NewClient(stub, stub)
	claimInterval, err := client.ClaimLeadership(StubServiceNm, StubUnitNm)

	c.Assert(err, gc.IsNil)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(claimInterval, gc.Equals, claimTime)
}

func (s *clientSuite) TestClaimLeadershipErrorTranslation(c *gc.C) {

	// First check translating errors embedded in the result.
	errMsg := "I'm trying!"
	numStubCalls := 0
	stub := &stubFacade{
		FacadeCallFn: func(name string, parameters, response interface{}) error {
			numStubCalls++
			typedR, ok := response.(*params.ClaimLeadershipBulkResults)
			c.Assert(ok, gc.Equals, true)
			typedR.Results = []params.ClaimLeadershipResults{params.ClaimLeadershipResults{
				Error: &params.Error{Message: errMsg},
			}}
			return nil
		},
	}

	client := NewClient(stub, stub)
	_, err := client.ClaimLeadership(StubServiceNm, StubUnitNm)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, gc.ErrorMatches, errMsg)

	// Now check errors returned from the function itself.
	errMsg = "well, I just give up."
	numStubCalls = 0
	stub.FacadeCallFn = func(name string, parameters, response interface{}) error {
		numStubCalls++
		return fmt.Errorf(errMsg)
	}

	_, err = client.ClaimLeadership(StubServiceNm, StubUnitNm)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, gc.ErrorMatches, "error making a leadership claim: "+errMsg)
}

func (s *clientSuite) TestReleaseLeadershipTranslation(c *gc.C) {

	numStubCalls := 0
	stub := &stubFacade{
		FacadeCallFn: func(name string, parameters, response interface{}) error {
			numStubCalls++
			c.Check(name, gc.Equals, "ReleaseLeadership")
			c.Assert(parameters, gc.Not(gc.IsNil))

			typedP, ok := parameters.(params.ReleaseLeadershipBulkParams)
			c.Assert(ok, gc.Equals, true)

			typedR, ok := response.(*params.ReleaseLeadershipBulkResults)
			c.Assert(ok, gc.Equals, true)
			typedR.Results = []params.ErrorResult{{}}

			c.Assert(typedP.Params, gc.HasLen, 1)
			c.Check(typedP.Params[0].ServiceTag.Id(), gc.Equals, StubServiceNm)
			c.Check(typedP.Params[0].UnitTag.Id(), gc.Equals, StubUnitNm)

			return nil
		},
	}

	client := NewClient(stub, stub)
	err := client.ReleaseLeadership(StubServiceNm, StubUnitNm)

	c.Check(numStubCalls, gc.Equals, 1)
	c.Assert(err, gc.IsNil)
}

func (s *clientSuite) TestBlockUntilLeadershipReleasedTranslation(c *gc.C) {

	numStubCalls := 0
	stub := &stubFacade{
		FacadeCallFn: func(name string, parameters, response interface{}) error {
			numStubCalls++
			c.Check(name, gc.Equals, "BlockUntilLeadershipReleased")
			c.Assert(parameters, gc.Not(gc.IsNil))

			typedP, ok := parameters.(names.ServiceTag)
			c.Assert(ok, gc.Equals, true)
			c.Check(typedP.Id(), gc.Equals, StubServiceNm)

			_, ok = response.(*params.ErrorResult)
			c.Assert(ok, gc.Equals, true)

			return nil
		},
	}

	client := NewClient(stub, stub)
	err := client.BlockUntilLeadershipReleased(StubServiceNm)

	c.Check(numStubCalls, gc.Equals, 1)
	c.Assert(err, gc.IsNil)
}
