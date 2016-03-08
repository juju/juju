// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agenttools"
	"github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/testing"
)

type AgentToolsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&AgentToolsSuite{})

func (s *AgentToolsSuite) TestUpdateToolsVersion(c *gc.C) {
	called := false
	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "AgentTools")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UpdateToolsAvailable")

			c.Assert(a, gc.IsNil)
			return nil
		})
	client := agenttools.NewFacade(apiCaller)
	err := client.UpdateToolsVersion()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
