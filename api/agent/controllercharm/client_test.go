// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm_test

import (
	"fmt"
	"testing"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/controllercharm"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func Test(t *testing.T) {
	gc.TestingT(t)
}

func (*suite) TestAddMetricsUserSuccess(c *gc.C) {
	username := "foo"
	password := "bar"

	client := newClient(func(objType string, version int, id, request string, args, results interface{}) error {
		c.Assert(objType, gc.Equals, "ControllerCharm")
		c.Assert(version, gc.Equals, 1)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "AddMetricsUser")
		c.Assert(args, gc.DeepEquals, params.AddUsers{[]params.AddUser{{
			Username:    username,
			DisplayName: username,
			Password:    password,
		}}})

		results.(*params.AddUserResults).Results = []params.AddUserResult{{
			Tag:   username,
			Error: nil,
		}}
		return nil
	})

	err := client.AddMetricsUser("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
}

func (*suite) TestAddMetricsUserFailure(c *gc.C) {
	username := "foo"
	password := "bar"
	errMsg := fmt.Sprintf("user %q already exists", username)

	client := newClient(func(objType string, version int, id, request string, args, results interface{}) error {
		c.Assert(objType, gc.Equals, "ControllerCharm")
		c.Assert(version, gc.Equals, 1)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "AddMetricsUser")
		c.Assert(args, gc.DeepEquals, params.AddUsers{[]params.AddUser{{
			Username:    username,
			DisplayName: username,
			Password:    password,
		}}})

		results.(*params.AddUserResults).Results = []params.AddUserResult{{
			Tag:   username,
			Error: &params.Error{Message: errMsg},
		}}
		return nil
	})

	err := client.AddMetricsUser("foo", "bar")
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("AddMetricsUser facade call failed: %s", errMsg))
}

func (*suite) TestRemoveMetricsUserSucceed(c *gc.C) {
	username := "foo"

	client := newClient(func(objType string, version int, id, request string, args, results interface{}) error {
		c.Assert(objType, gc.Equals, "ControllerCharm")
		c.Assert(version, gc.Equals, 1)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "RemoveMetricsUser")
		c.Assert(args, gc.DeepEquals, params.Entities{[]params.Entity{{
			Tag: names.NewUserTag(username).String(),
		}}})

		results.(*params.ErrorResults).Results = []params.ErrorResult{{
			Error: nil,
		}}
		return nil
	})

	err := client.RemoveMetricsUser("foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (*suite) TestRemoveMetricsUserFailure(c *gc.C) {
	username := "foo"
	errMsg := fmt.Sprintf(`username %q should have prefix "juju-metrics-"`, username)

	client := newClient(func(objType string, version int, id, request string, args, results interface{}) error {
		c.Assert(objType, gc.Equals, "ControllerCharm")
		c.Assert(version, gc.Equals, 1)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "RemoveMetricsUser")
		c.Assert(args, gc.DeepEquals, params.Entities{[]params.Entity{{
			Tag: names.NewUserTag(username).String(),
		}}})

		results.(*params.ErrorResults).Results = []params.ErrorResult{{
			Error: &params.Error{Message: errMsg},
		}}
		return nil
	})

	err := client.RemoveMetricsUser("foo")
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("RemoveMetricsUser facade call failed: %s", errMsg))
}

func newClient(f basetesting.APICallerFunc) *controllercharm.Client {
	return controllercharm.NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}
