// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource/resourceadapters"
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
	msg := "trust"
	attempts := 0
	s.resourceClient.getResourceF = func(req charmstore.ResourceRequest) (data charmstore.ResourceData, err error) {
		attempts++
		return charmstore.ResourceData{}, errors.New(msg)
	}
	csRes := resourceadapters.NewCSRetryClientForTest(s.resourceClient)

	_, err := csRes.GetResource(charmstore.ResourceRequest{})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("failed after retrying: %v", msg))
	// Ensure we logged attempts @ WARNING.
	c.Assert(c.GetTestLog(), jc.Contains, fmt.Sprintf("WARNING juju.resource.resourceadapters (attempt %v) retrying resource ", attempts))

	callsMade := []string{}
	for i := 0; i < attempts; i++ {
		callsMade = append(callsMade, "GetResource")
	}
	s.resourceClient.stub.CheckCallNames(c, callsMade...)
}

func (s *CharmStoreSuite) TestGetResourceAbortedOnNotFound(c *gc.C) {
	msg := "trust"
	s.resourceClient.getResourceF = func(req charmstore.ResourceRequest) (data charmstore.ResourceData, err error) {
		return charmstore.ResourceData{}, errors.NotFoundf(msg)
	}
	s.assertAbortedGetResource(c,
		resourceadapters.NewCSRetryClientForTest(s.resourceClient),
		fmt.Sprintf("%v not found", msg),
	)
}

func (s *CharmStoreSuite) TestGetResourceAbortedOnNotValid(c *gc.C) {
	msg := "trust"
	s.resourceClient.getResourceF = func(req charmstore.ResourceRequest) (data charmstore.ResourceData, err error) {
		return charmstore.ResourceData{}, errors.NotValidf(msg)
	}
	s.assertAbortedGetResource(c,
		resourceadapters.NewCSRetryClientForTest(s.resourceClient),
		fmt.Sprintf("%v not valid", msg),
	)
}

func (s *CharmStoreSuite) assertAbortedGetResource(c *gc.C, csRes *resourceadapters.CSRetryClient, expectedError string) {
	_, err := csRes.GetResource(charmstore.ResourceRequest{})
	c.Assert(err, gc.ErrorMatches, expectedError)
	c.Assert(c.GetTestLog(), gc.Not(jc.Contains), "WARNING juju.resource.resourceadapters")
	s.resourceClient.stub.CheckCallNames(c, "GetResource")
}

type testResourceClient struct {
	stub *testing.Stub

	getResourceF func(req charmstore.ResourceRequest) (data charmstore.ResourceData, err error)
}

func (f *testResourceClient) GetResource(req charmstore.ResourceRequest) (data charmstore.ResourceData, err error) {
	f.stub.AddCall("GetResource", req)
	return f.getResourceF(req)
}
