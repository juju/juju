// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setmeterstatus_test

import (
	stdtesting "testing"

	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/cmd/juju/setmeterstatus"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type SetMeterStatusSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&SetMeterStatusSuite{})

func (s *SetMeterStatusSuite) TestDebugNoArgs(c *gc.C) {
	cmd := setmeterstatus.NewCommandForTest(nil)
	_, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, `you need to specify an entity \(application or unit\) and a status`)
}

func (s *SetMeterStatusSuite) TestUnits(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "MetricsDebug")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetMeterStatus")
		c.Assert(arg, jc.DeepEquals, params.MeterStatusParams{
			Statuses: []params.MeterStatusParam{{
				Tag:  "unit-mysql-0",
				Code: "RED",
				Info: "foobar",
			},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	cmd := setmeterstatus.NewCommandForTest(apiCaller)
	_, err := cmdtesting.RunCommand(c, cmd, "mysql/0", "RED", "--info", "foobar")
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SetMeterStatusSuite) TestApplication(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "MetricsDebug")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetMeterStatus")
		c.Assert(arg, jc.DeepEquals, params.MeterStatusParams{
			Statuses: []params.MeterStatusParam{{
				Tag:  "application-mysql",
				Code: "RED",
				Info: "foobar",
			},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	cmd := setmeterstatus.NewCommandForTest(apiCaller)
	_, err := cmdtesting.RunCommand(c, cmd, "mysql", "RED", "--info", "foobar")
	c.Assert(err, gc.ErrorMatches, "FAIL")
}
