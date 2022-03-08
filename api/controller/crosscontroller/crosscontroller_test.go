// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/crosscontroller"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CrossControllerSuite{})

type CrossControllerSuite struct {
	coretesting.BaseSuite
}

func (s *CrossControllerSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := crosscontroller.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *CrossControllerSuite) TestControllerInfo(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossController")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ControllerInfo")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.ControllerAPIInfoResults{})
		*(result.(*params.ControllerAPIInfoResults)) = params.ControllerAPIInfoResults{
			[]params.ControllerAPIInfoResult{{
				Addresses: []string{"foo"},
				CACert:    "bar",
			}},
		}
		return nil
	})
	client := crosscontroller.NewClient(apiCaller)
	info, err := client.ControllerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &crosscontroller.ControllerInfo{
		Addrs:  []string{"foo"},
		CACert: "bar",
	})
}

func (s *CrossControllerSuite) TestControllerInfoError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ControllerAPIInfoResults)) = params.ControllerAPIInfoResults{
			[]params.ControllerAPIInfoResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := crosscontroller.NewClient(apiCaller)
	info, err := client.ControllerInfo()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(info, gc.IsNil)
}

func (s *CrossControllerSuite) TestWatchExternalControllers(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossController")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchControllerInfo")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			[]params.NotifyWatchResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := crosscontroller.NewClient(apiCaller)
	w, err := client.WatchControllerInfo()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(w, gc.IsNil)
}
