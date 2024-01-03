// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type slaSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&slaSuite{})

func (s *slaSuite) TestSLALevel(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "SLALevel")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringResult{})
		*(result.(*params.StringResult)) = params.StringResult{
			Result: "essential",
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	level, err := client.SLALevel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(level, gc.Equals, "essential")
}
