// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"fmt"
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
)

type CharmStoreSuite struct {
	testing.IsolationSuite

	resourceClient *testResourceClient
}

var _ = gc.Suite(&CharmStoreSuite{})

func (s *CharmStoreSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.resourceClient = &testResourceClient{
		stub: &testing.Stub{},
	}
}

func (s *CharmStoreSuite) TestGetResourceTerminates(c *gc.C) {
	s.testGetResourceTerminates(c, true)
}

func (s *CharmStoreSuite) TestGetResourceTerminatesNoChannel(c *gc.C) {
	s.testGetResourceTerminates(c, false)
}

func (s *CharmStoreSuite) testGetResourceTerminates(c *gc.C, channel bool) {
	msg := "trust"
	attempts := int32(0)
	s.resourceClient.getResourceF = func(req resource.ResourceRequest) (data resource.ResourceData, err error) {
		atomic.AddInt32(&attempts, 1)
		return resource.ResourceData{}, errors.New(msg)
	}
	csRes := resource.NewCSRetryClientForTest(s.resourceClient)

	req := resource.ResourceRequest{}
	if channel {
		req.CharmID.Origin.Channel = &state.Channel{Risk: "stable"}
	}
	_, err := csRes.GetResource(req)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("failed after retrying: %v", msg))
	// Ensure we logged attempts @ WARNING.
	//c.Assert(c.GetTestLog(), jc.Contains, fmt.Sprintf("WARNING juju.resource attempt %d/%d to download resource ", attempts, attempts))

	callsMade := []string{}
	for i := int32(0); i < attempts; i++ {
		callsMade = append(callsMade, "GetResource")
	}
	c.Assert(attempts, jc.GreaterThan, 1)
	s.resourceClient.stub.CheckCallNames(c, callsMade...)
}

func (s *CharmStoreSuite) TestGetResourceAbortedOnNotFound(c *gc.C) {
	msg := "trust"
	s.assertAbortedGetResourceOnError(c,
		resource.NewCSRetryClientForTest(s.resourceClient),
		errors.NotFoundf(msg),
		fmt.Sprintf("%v not found", msg),
	)
}

func (s *CharmStoreSuite) TestGetResourceAbortedOnNotValid(c *gc.C) {
	msg := "trust"
	s.assertAbortedGetResourceOnError(c,
		resource.NewCSRetryClientForTest(s.resourceClient),
		errors.NotValidf(msg),
		fmt.Sprintf("%v not valid", msg),
	)
}

func (s *CharmStoreSuite) assertAbortedGetResourceOnError(c *gc.C, csRes *resource.ResourceRetryClient, expectedError error, expectedMessage string) {
	s.resourceClient.getResourceF = func(req resource.ResourceRequest) (data resource.ResourceData, err error) {
		return resource.ResourceData{}, expectedError
	}
	_, err := csRes.GetResource(resource.ResourceRequest{
		CharmID: resource.CharmID{
			URL: nil,
			Origin: state.CharmOrigin{
				Channel: &state.Channel{Risk: "stable"},
			},
		},
	})
	c.Assert(err, gc.ErrorMatches, expectedMessage)
	//c.Assert(c.GetTestLog(), gc.Not(jc.Contains), "WARNING juju.resource")
	// Since we have aborted re-tries, we should only call GetResources once.
	s.resourceClient.stub.CheckCallNames(c, "GetResource")
}

type testResourceClient struct {
	stub *testing.Stub

	getResourceF func(req resource.ResourceRequest) (data resource.ResourceData, err error)
}

func (f *testResourceClient) GetResource(req resource.ResourceRequest) (data resource.ResourceData, err error) {
	f.stub.AddCall("GetResource", req)
	return f.getResourceF(req)
}
