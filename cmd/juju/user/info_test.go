// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package user_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/api/client/usermanager"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

var logger = internallogger.GetLogger("juju.cmd.user.test")

// All of the functionality of the UserInfo api call is contained elsewhere.
// This suite provides basic tests for the "show-user" command
type UserInfoCommandSuite struct {
	BaseSuite
}

func TestUserInfoCommandSuite(t *stdtesting.T) {
	tc.Run(t, &UserInfoCommandSuite{})
}

var (

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

func (*fakeUserInfoAPI) UserInfo(ctx context.Context, usernames []string, all usermanager.IncludeDisabled) ([]params.UserInfo, error) {
	logger.Infof(context.TODO(), "fakeUserInfoAPI.UserInfo(%v, %v)", usernames, all)
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

func (s *UserInfoCommandSuite) TestUserInfo(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `user-name: current-user
access: add-model
date-created: "1981-02-27"
last-connection: "2014-01-01"
`)
}

func (s *UserInfoCommandSuite) TestUserInfoExactTime(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "--exact-time")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `user-name: current-user
access: add-model
date-created: 1981-02-27 16:10:05 +0000 UTC
last-connection: 2014-01-01 00:00:00 +0000 UTC
`)
}

func (s *UserInfoCommandSuite) TestUserInfoWithUsername(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "foobar")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `user-name: foobar
display-name: Foo Bar
access: login
date-created: "1981-02-27"
last-connection: "2014-01-01"
`)
}

func (s *UserInfoCommandSuite) TestUserInfoExternalUser(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "fred@external")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `user-name: fred@external
display-name: Fred External
access: add-model
`)
}

func (s *UserInfoCommandSuite) TestUserInfoUserDoesNotExist(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "barfoo")
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *UserInfoCommandSuite) TestUserInfoFormatJson(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `
{"user-name":"current-user","access":"add-model","date-created":"1981-02-27","last-connection":"2014-01-01"}
`[1:])
}

func (s *UserInfoCommandSuite) TestUserInfoFormatJsonWithUsername(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "foobar", "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `
{"user-name":"foobar","display-name":"Foo Bar","access":"login","date-created":"1981-02-27","last-connection":"2014-01-01"}
`[1:])
}

func (s *UserInfoCommandSuite) TestUserInfoFormatYaml(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `user-name: current-user
access: add-model
date-created: "1981-02-27"
last-connection: "2014-01-01"
`)
}

func (s *UserInfoCommandSuite) TestTooManyArgs(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.NewShowUserCommand(), "username", "whoops")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
