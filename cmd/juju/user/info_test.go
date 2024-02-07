// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package user_test

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/usermanager"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.cmd.user.test")

// All of the functionality of the UserInfo api call is contained elsewhere.
// This suite provides basic tests for the "show-user" command
type UserInfoCommandSuite struct {
	BaseSuite
}

var (
	_ = gc.Suite(&UserInfoCommandSuite{})

	// Mock out timestamps
	dateCreated    = time.Unix(352138205, 0).UTC()
	lastConnection = time.Unix(1388534400, 0).UTC()
)

func (s *UserInfoCommandSuite) NewShowUserCommand() cmd.Command {
	return user.NewShowUserCommandForTest(&fakeUserInfoAPI{}, s.store)
}

type fakeUserInfoAPI struct{}

func (*fakeUserInfoAPI) Close() error {
	return nil
}

func (*fakeUserInfoAPI) UserInfo(usernames []string, all usermanager.IncludeDisabled) ([]params.UserInfo, error) {
	logger.Infof("fakeUserInfoAPI.UserInfo(%v, %v)", usernames, all)
	info := params.UserInfo{
		DateCreated:    dateCreated,
		LastConnection: &lastConnection,
	}
	switch usernames[0] {
	case "current-user":
		info.Username = "current-user"
		info.Access = "add-model"
	case "foobar":
		info.Username = "foobar"
		info.DisplayName = "Foo Bar"
		info.Access = "login"
	case "fred@external":
		info.Username = "fred@external"
		info.DisplayName = "Fred External"
		info.Access = "add-model"
	default:
		return nil, apiservererrors.ErrPerm
	}
	return []params.UserInfo{info}, nil
}

func (s *UserInfoCommandSuite) TestUserInfo(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `user-name: current-user
access: add-model
date-created: "1981-02-27"
last-connection: "2014-01-01"
`)
}

func (s *UserInfoCommandSuite) TestUserInfoExactTime(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "--exact-time")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `user-name: current-user
access: add-model
date-created: 1981-02-27 16:10:05 +0000 UTC
last-connection: 2014-01-01 00:00:00 +0000 UTC
`)
}

func (s *UserInfoCommandSuite) TestUserInfoWithUsername(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `user-name: foobar
display-name: Foo Bar
access: login
date-created: "1981-02-27"
last-connection: "2014-01-01"
`)
}

func (s *UserInfoCommandSuite) TestUserInfoExternalUser(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "fred@external")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `user-name: fred@external
display-name: Fred External
access: add-model
`)
}

func (s *UserInfoCommandSuite) TestUserInfoUserDoesNotExist(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "barfoo")
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *UserInfoCommandSuite) TestUserInfoFormatJson(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
{"user-name":"current-user","access":"add-model","date-created":"1981-02-27","last-connection":"2014-01-01"}
`[1:])
}

func (s *UserInfoCommandSuite) TestUserInfoFormatJsonWithUsername(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "foobar", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
{"user-name":"foobar","display-name":"Foo Bar","access":"login","date-created":"1981-02-27","last-connection":"2014-01-01"}
`[1:])
}

func (s *UserInfoCommandSuite) TestUserInfoFormatYaml(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `user-name: current-user
access: add-model
date-created: "1981-02-27"
last-connection: "2014-01-01"
`)
}

func (s *UserInfoCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "username", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
