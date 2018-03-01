// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/caasagent"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&caasagentSuite{})

type caasagentSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     *caasagent.Facade
	st         *mockState
}

func (s *caasagentSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}

	s.st = &mockState{}
	model, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)

	facade, err := caasagent.NewFacade(s.resources, s.authorizer, s.st, nil, model)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
}

func (s *caasagentSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("someapp"),
	}
	_, err := caasagent.NewFacade(s.resources, s.authorizer, s.st, nil, nil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *caasagentSuite) TestModel(c *gc.C) {
	result, err := s.facade.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.Model{
		Name:     "some-model",
		UUID:     coretesting.ModelTag.Id(),
		Type:     "caas",
		OwnerTag: "user-fred",
	})

	s.st.CheckCallNames(c, "Model")
}
