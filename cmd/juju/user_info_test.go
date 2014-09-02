// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package main

import (
	"time"

	"github.com/juju/cmd"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/usermanager"
	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

// All of the functionality of the UserInfo api call is contained elsewhere.
// This suite provides basic tests for the "user info" command
type UserInfoCommandSuite struct {
	jujutesting.JujuConnSuite
}

var (
	_ = gc.Suite(&UserInfoCommandSuite{})

	// Mock out timestamps
	dateCreated    = time.Unix(352138205, 0).UTC()
	lastConnection = time.Unix(1388534400, 0).UTC()
)

func (s *UserInfoCommandSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(&getUserInfoAPI, func(*UserInfoCommand) (UserInfoAPI, error) {
		return &fakeUserInfoAPI{}, nil
	})
}

func newUserInfoCommand() cmd.Command {
	return envcmd.Wrap(&UserInfoCommand{})
}

type fakeUserInfoAPI struct {
	UserInfoCommandSuite
}

func (*fakeUserInfoAPI) Close() error {
	return nil
}

func (f *fakeUserInfoAPI) UserInfo(username string) (result usermanager.UserInfoResult, err error) {
	info := usermanager.UserInfo{
		DateCreated:    dateCreated,
		LastConnection: &lastConnection,
	}
	switch username {
	case "admin":
		info.Username = "admin"
	case "foobar":
		info.Username = "foobar"
		info.DisplayName = "Foo Bar"
	default:
		return usermanager.UserInfoResult{}, common.ErrPerm
	}
	result.Result = &info
	return result, nil
}

func (s *UserInfoCommandSuite) TestUserInfo(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand())
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `user-name: admin
display-name: ""
date-created: `+dateCreated.String()+`
last-connection: `+lastConnection.String()+"\n")
}

func (s *UserInfoCommandSuite) TestUserInfoWithUsername(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand(), "foobar")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `user-name: foobar
display-name: Foo Bar
date-created: `+dateCreated.String()+`
last-connection: `+lastConnection.String()+"\n")
}

func (*UserInfoCommandSuite) TestUserInfoUserDoesNotExist(c *gc.C) {
	_, err := testing.RunCommand(c, newUserInfoCommand(), "barfoo")
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (*UserInfoCommandSuite) TestUserInfoFormatJson(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand(), "--format", "json")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
{"user-name":"admin","display-name":"","date-created":"1981-02-27 16:10:05 +0000 UTC","last-connection":"2014-01-01 00:00:00 +0000 UTC"}
`[1:])
}

func (*UserInfoCommandSuite) TestUserInfoFormatJsonWithUsername(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand(), "foobar", "--format", "json")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
{"user-name":"foobar","display-name":"Foo Bar","date-created":"1981-02-27 16:10:05 +0000 UTC","last-connection":"2014-01-01 00:00:00 +0000 UTC"}
`[1:])
}

func (*UserInfoCommandSuite) TestUserInfoFormatYaml(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand(), "--format", "yaml")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `user-name: admin
display-name: ""
date-created: `+dateCreated.String()+`
last-connection: `+lastConnection.String()+"\n")
}

func (*UserInfoCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserInfoCommand(), "username", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
