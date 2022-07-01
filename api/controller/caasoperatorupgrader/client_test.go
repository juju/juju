// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/v3/api/base/testing"
	"github.com/juju/juju/v3/api/controller/caasoperatorupgrader"
	"github.com/juju/juju/v3/rpc/params"
)

type provisionerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&provisionerSuite{})

func newClient(f basetesting.APICallerFunc) *caasoperatorupgrader.Client {
	return caasoperatorupgrader.NewClient(basetesting.BestVersionCaller{f, 5})
}

func (s *provisionerSuite) TestUpgrader(c *gc.C) {
	var called bool
	client := newClient(func(objType string, v int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASOperatorUpgrader")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "UpgradeOperator")
		c.Assert(a, jc.DeepEquals, params.KubernetesUpgradeArg{
			AgentTag: "application-foo",
			Version:  version.MustParse("6.6.6"),
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResult{})
		*(result.(*params.ErrorResult)) = params.ErrorResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	err := client.Upgrade("application-foo", version.MustParse("6.6.6"))
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(called, jc.IsTrue)
}
