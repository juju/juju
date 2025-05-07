// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller_test

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/crosscontroller"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&CrossControllerSuite{})

type CrossControllerSuite struct {
	coretesting.BaseSuite
}

func (s *CrossControllerSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := crosscontroller.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func (s *CrossControllerSuite) TestControllerInfo(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossController")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ControllerInfo")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.ControllerAPIInfoResults{})
		*(result.(*params.ControllerAPIInfoResults)) = params.ControllerAPIInfoResults{
			Results: []params.ControllerAPIInfoResult{{
				Addresses: []string{"foo"},
				CACert:    "bar",
			}},
		}
		return nil
	})
	client := crosscontroller.NewClient(apiCaller)
	info, err := client.ControllerInfo(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, &crosscontroller.ControllerInfo{
		Addrs:  []string{"foo"},
		CACert: "bar",
	})
}

func (s *CrossControllerSuite) TestControllerInfoError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ControllerAPIInfoResults)) = params.ControllerAPIInfoResults{
			Results: []params.ControllerAPIInfoResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := crosscontroller.NewClient(apiCaller)
	info, err := client.ControllerInfo(context.Background())
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Assert(info, tc.IsNil)
}

func (s *CrossControllerSuite) TestWatchExternalControllers(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossController")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchControllerInfo")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := crosscontroller.NewClient(apiCaller)
	w, err := client.WatchControllerInfo(context.Background())
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Assert(w, tc.IsNil)
}
