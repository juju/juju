// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	"github.com/juju/names/v6"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/caasagent"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	modeltesting "github.com/juju/juju/core/model/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

var _ = gc.Suite(&caasagentSuite{})

type caasagentSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
}

func (s *caasagentSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
}

func (s *caasagentSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("someapp"),
	}
	modelUUID := modeltesting.GenModelUUID(c)

	_, err := caasagent.NewStateFacadeV2(facadetest.ModelContext{
		Auth_:      s.authorizer,
		ModelUUID_: modelUUID,
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
