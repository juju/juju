// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasapplication"
	"github.com/juju/juju/apiserver/params"
)

type provisionerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&provisionerSuite{})

func newClient(f basetesting.APICallerFunc) *caasapplication.Client {
	return caasapplication.NewClient(basetesting.BestVersionCaller{f, 1})
}

func (s *provisionerSuite) TestUnitIntroduction(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Assert(objType, gc.Equals, "CAASApplication")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "UnitIntroduction")
		c.Assert(a, gc.FitsTypeOf, params.CAASUnitIntroductionArgs{})
		args := a.(params.CAASUnitIntroductionArgs)
		c.Assert(args.PodName, gc.Equals, "pod-name")
		c.Assert(args.PodUUID, gc.Equals, "pod-uuid")
		c.Assert(result, gc.FitsTypeOf, &params.CAASUnitIntroductionResult{})
		*(result.(*params.CAASUnitIntroductionResult)) = params.CAASUnitIntroductionResult{
			Result: &params.CAASUnitIntroduction{
				AgentConf: []byte("config data"),
				UnitName:  "app/0",
			},
		}
		return nil
	})
	unitConfig, err := client.UnitIntroduction("pod-name", "pod-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(unitConfig, gc.NotNil)
	c.Assert(unitConfig.UnitTag.String(), gc.Equals, "unit-app-0")
	c.Assert(unitConfig.AgentConf, jc.SameContents, []byte("config data"))
}

func (s *provisionerSuite) TestUnitIntroductionFail(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Assert(objType, gc.Equals, "CAASApplication")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "UnitIntroduction")
		c.Assert(a, gc.FitsTypeOf, params.CAASUnitIntroductionArgs{})
		args := a.(params.CAASUnitIntroductionArgs)
		c.Assert(args.PodName, gc.Equals, "pod-name")
		c.Assert(args.PodUUID, gc.Equals, "pod-uuid")
		c.Assert(result, gc.FitsTypeOf, &params.CAASUnitIntroductionResult{})
		*(result.(*params.CAASUnitIntroductionResult)) = params.CAASUnitIntroductionResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	_, err := client.UnitIntroduction("pod-name", "pod-uuid")
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(called, jc.IsTrue)
}
