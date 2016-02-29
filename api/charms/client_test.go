// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type charmsMockSuite struct {
	coretesting.BaseSuite
	charmsClient *charms.Client
}

//TODO (mattyw) There are just mock tests in here. We need real tests for each api call.

var _ = gc.Suite(&charmsMockSuite{})

func (s *charmsMockSuite) TestIsMeteredFalse(c *gc.C) {
	var called bool
	curl := "local:quantal/dummy-1"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Charms")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "IsMetered")

			args, ok := a.(params.CharmInfo)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.CharmURL, gc.DeepEquals, curl)
			return nil
		})
	charmsClient := charms.NewClient(apiCaller)
	_, err := charmsClient.IsMetered(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
