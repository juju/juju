// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !ppc64

package reboot_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/reboot"
	"github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)


type machineRebootSuite struct {
	coretesting.BaseSuite
}

func TestMachineRebootSuite(t *stdtesting.T) { tc.Run(t, &machineRebootSuite{}) }
func (s *machineRebootSuite) TestWatchForRebootEvent(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Reboot")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchForRebootEvent")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	_, err := client.WatchForRebootEvent(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *machineRebootSuite) TestRequestReboot(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Reboot")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RequestReboot")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	err := client.RequestReboot(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *machineRebootSuite) TestRequestRebootError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Reboot")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RequestReboot")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "FAIL"}}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	err := client.RequestReboot(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *machineRebootSuite) TestGetRebootAction(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Reboot")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetRebootAction")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.RebootActionResults{})
		*(result.(*params.RebootActionResults)) = params.RebootActionResults{
			Results: []params.RebootActionResult{{
				Result: params.ShouldDoNothing,
			}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	rAction, err := client.GetRebootAction(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rAction, tc.Equals, params.ShouldDoNothing)
}

func (s *machineRebootSuite) TestGetRebootActionMultipleResults(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Reboot")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetRebootAction")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.RebootActionResults{})
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
	_, err := client.GetRebootAction(c.Context())
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *machineRebootSuite) TestClearReboot(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Reboot")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ClearReboot")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	err := client.ClearReboot(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *machineRebootSuite) TestClearRebootError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Reboot")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ClearReboot")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "FAIL"}}}}
		return nil

	})
	tag := names.NewMachineTag("666")
	client := reboot.NewClient(apiCaller, tag)
	err := client.ClearReboot(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
}
