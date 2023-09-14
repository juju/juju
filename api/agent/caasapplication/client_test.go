// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/caasapplication"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
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

func (s *provisionerSuite) TestUnitIntroductionFailAlreadyExists(c *gc.C) {
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
			Error: &params.Error{Code: params.CodeAlreadyExists},
		}
		return nil
	})
	_, err := client.UnitIntroduction("pod-name", "pod-uuid")
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
	c.Assert(called, jc.IsTrue)
}

func (s *provisionerSuite) TestUnitIntroductionFailNotAssigned(c *gc.C) {
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
			Error: &params.Error{Code: params.CodeNotAssigned},
		}
		return nil
	})
	_, err := client.UnitIntroduction("pod-name", "pod-uuid")
	c.Assert(err, jc.ErrorIs, errors.NotAssigned)
	c.Assert(called, jc.IsTrue)
}

func (s *provisionerSuite) TestUnitTerminating(c *gc.C) {
	tests := []struct {
		willRestart bool
		err         error
	}{
		{false, nil},
		{true, nil},
		{false, errors.New("oops")},
	}
	for _, test := range tests {
		var called bool
		client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
			called = true
			c.Assert(objType, gc.Equals, "CAASApplication")
			c.Assert(id, gc.Equals, "")
			c.Assert(request, gc.Equals, "UnitTerminating")
			c.Assert(a, gc.FitsTypeOf, params.Entity{})
			args := a.(params.Entity)
			c.Assert(args.Tag, gc.Equals, "unit-app-0")
			c.Assert(result, gc.FitsTypeOf, &params.CAASUnitTerminationResult{})
			var err *params.Error
			if test.err != nil {
				err = &params.Error{Message: test.err.Error()}
			}
			*(result.(*params.CAASUnitTerminationResult)) = params.CAASUnitTerminationResult{
				WillRestart: test.willRestart,
				Error:       err,
			}
			return nil
		})
		unitTermination, err := client.UnitTerminating(names.NewUnitTag("app/0"))
		if test.err == nil {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err.Error())
		}
		c.Assert(called, jc.IsTrue)
		c.Assert(unitTermination, gc.DeepEquals, caasapplication.UnitTermination{WillRestart: test.willRestart})
	}
}
