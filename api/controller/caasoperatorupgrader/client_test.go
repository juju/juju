// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasoperatorupgrader"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type provisionerSuite struct {
	testhelpers.IsolationSuite
}

func TestProvisionerSuite(t *stdtesting.T) {
	tc.Run(t, &provisionerSuite{})
}

func newClient(f basetesting.APICallerFunc) *caasoperatorupgrader.Client {
	return caasoperatorupgrader.NewClient(basetesting.BestVersionCaller{f, 5})
}

func (s *provisionerSuite) TestUpgrader(c *tc.C) {
	var called bool
	client := newClient(func(objType string, v int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASOperatorUpgrader")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "UpgradeOperator")
		c.Assert(a, tc.DeepEquals, params.KubernetesUpgradeArg{
			AgentTag: "application-foo",
			Version:  semversion.MustParse("6.6.6"),
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResult{})
		*(result.(*params.ErrorResult)) = params.ErrorResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	err := client.Upgrade(c.Context(), "application-foo", semversion.MustParse("6.6.6"))
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(called, tc.IsTrue)
}
