// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type slaSuiteV4 struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&slaSuiteV4{})

func (s *slaSuiteV4) TestSetPodSpecApplication(c *gc.C) {
	c.Skip("this API not present in V4")
}

func (s *slaSuiteV4) TestSLALevelOldFacadeVersion(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 4}
	st := uniter.NewState(caller, names.NewUnitTag("wordpress/0"))
	level, err := st.SLALevel()
	c.Assert(err, jc.ErrorIsNil)

	// testing.APICallerFunc declared the BestFacadeVersion to be 0: that is why we
	// expect "unsupported", because we are talking to an old apiserver.
	c.Assert(level, gc.Equals, "unsupported")
}

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
	caller := testing.BestVersionCaller{apiCaller, 5}
	client := uniter.NewState(caller, names.NewUnitTag("mysql/0"))
	level, err := client.SLALevel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(level, gc.Equals, "essential")
}
