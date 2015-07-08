// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/spaces"
	ast "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type SpacesSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer ast.FakeAuthorizer
	facade     spaces.API
}

var _ = gc.Suite(&SpacesSuite{})

func (s *SpacesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	BackingInstance.SetUp(c, StubZonedEnvironName, WithZones, WithSpaces)

	s.resources = common.NewResources()
	s.authorizer = ast.FakeAuthorizer{
		Tag:            names.NewUserTag("admin"),
		EnvironManager: false,
	}

	var err error
	s.facade, err = spaces.NewAPI(BackingInstance, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.facade, gc.NotNil)
}

func (s *SpacesSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *SpacesSuite) TestNewAPI(c *gc.C) {
	// Clients are allowed.
	facade, err := spaces.NewAPI(BackingInstance, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(facade, gc.NotNil)
	// No calls so far.
	CheckMethodCalls(c, SharedStub)

	// Agents are not allowed
	agentAuthorizer := s.authorizer
	agentAuthorizer.Tag = names.NewMachineTag("42")
	facade, err = spaces.NewAPI(BackingInstance, s.resources, agentAuthorizer)
	c.Assert(err, jc.DeepEquals, common.ErrPerm)
	c.Assert(facade, gc.IsNil)
	// No calls so far.
	CheckMethodCalls(c, SharedStub)
}
