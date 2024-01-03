// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !ppc64

package reboot_test

import (
	stdtesting "testing"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/reboot"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type machineRebootSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&machineRebootSuite{})

func (s *machineRebootSuite) TestWatchForRebootEvent(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Reboot")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchForRebootEvent")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	_, err := client.WatchForRebootEvent()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *machineRebootSuite) TestRequestReboot(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Reboot")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RequestReboot")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	err := client.RequestReboot()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machineRebootSuite) TestRequestRebootError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Reboot")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RequestReboot")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "FAIL"}}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	err := client.RequestReboot()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *machineRebootSuite) TestGetRebootAction(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Reboot")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetRebootAction")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.RebootActionResults{})
		*(result.(*params.RebootActionResults)) = params.RebootActionResults{
			Results: []params.RebootActionResult{{
				Result: params.ShouldDoNothing,
			}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	rAction, err := client.GetRebootAction()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAction, gc.Equals, params.ShouldDoNothing)
}

func (s *machineRebootSuite) TestGetRebootActionMultipleResults(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Reboot")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetRebootAction")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.RebootActionResults{})
		*(result.(*params.RebootActionResults)) = params.RebootActionResults{
			Results: []params.RebootActionResult{{
				Result: params.ShouldDoNothing,
			}, {
				Result: params.ShouldDoNothing,
			}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	_, err := client.GetRebootAction()
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *machineRebootSuite) TestClearReboot(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Reboot")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ClearReboot")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	err := client.ClearReboot()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machineRebootSuite) TestClearRebootError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Reboot")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ClearReboot")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "FAIL"}}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	err := client.ClearReboot()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}
