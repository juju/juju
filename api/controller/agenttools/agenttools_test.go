// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/agenttools"
	coretesting "github.com/juju/juju/internal/testing"
)

type AgentToolsSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&AgentToolsSuite{})

func (s *AgentToolsSuite) TestUpdateToolsVersion(c *tc.C) {
	called := false
	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, tc.Equals, "AgentTools")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "UpdateToolsAvailable")

			c.Assert(a, tc.IsNil)
			return nil
		})
	client := agenttools.NewFacade(apiCaller)
	err := client.UpdateToolsVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}
