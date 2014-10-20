// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package user_test

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/testing"
)

// All of the functionality of the UserInfo api call is contained elsewhere.
// This suite provides basic tests for the "user info" command
type UserInfoCommandSuite struct {
	BaseSuite
	logger loggo.Logger
}

var (
	_ = gc.Suite(&UserInfoCommandSuite{})

	// Mock out timestamps
	dateCreated    = time.Unix(352138205, 0).UTC()
	lastConnection = time.Unix(1388534400, 0).UTC()
)

func (s *UserInfoCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(user.GetUserInfoAPI, func(*user.InfoCommand) (user.UserInfoAPI, error) {
		return &fakeUserInfoAPI{s}, nil
	})
	s.logger = loggo.GetLogger("juju.user-info-test")
}

func newUserInfoCommand() cmd.Command {
	return envcmd.Wrap(&user.InfoCommand{})
}

type fakeUserInfoAPI struct {
	*UserInfoCommandSuite
}

func (*fakeUserInfoAPI) Close() error {
	return nil
}

func (f *fakeUserInfoAPI) UserInfo(usernames []string, all usermanager.IncludeDisabled) ([]params.UserInfo, error) {
	f.logger.Infof("fakeUserInfoAPI.UserInfo(%v, %v)", usernames, all)
	info := params.UserInfo{
		DateCreated:    dateCreated,
		LastConnection: &lastConnection,
	}
	switch usernames[0] {
	case "user-test":
		info.Username = "user-test"
	case "foobar":
		info.Username = "foobar"
		info.DisplayName = "Foo Bar"
	default:
		return nil, common.ErrPerm
	}
	return []params.UserInfo{info}, nil
}

func (s *UserInfoCommandSuite) TestUserInfo(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand())
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `user-name: user-test
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
{"user-name":"user-test","display-name":"","date-created":"1981-02-27 16:10:05 +0000 UTC","last-connection":"2014-01-01 00:00:00 +0000 UTC"}
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
	c.Assert(testing.Stdout(context), gc.Equals, `user-name: user-test
display-name: ""
date-created: `+dateCreated.String()+`
last-connection: `+lastConnection.String()+"\n")
}

func (*UserInfoCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserInfoCommand(), "username", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
