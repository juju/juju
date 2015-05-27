// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package user_test

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/testing"
)

var logger = loggo.GetLogger("juju.cmd.user.test")

// All of the functionality of the UserInfo api call is contained elsewhere.
// This suite provides basic tests for the "user info" command
type UserInfoCommandSuite struct {
	BaseSuite
}

var (
	_ = gc.Suite(&UserInfoCommandSuite{})

	// Mock out timestamps
	dateCreated    = time.Unix(352138205, 0).UTC()
	lastConnection = time.Unix(1388534400, 0).UTC()
)

func newUserInfoCommand() cmd.Command {
	return envcmd.WrapSystem(user.NewInfoCommand(&fakeUserInfoAPI{}))
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `user-name: user-test
display-name: ""
date-created: 1981-02-27
last-connection: 2014-01-01
`)
}

func (s *UserInfoCommandSuite) TestUserInfoExactTime(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand(), "--exact-time")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `user-name: user-test
display-name: ""
date-created: 1981-02-27 16:10:05 +0000 UTC
last-connection: 2014-01-01 00:00:00 +0000 UTC
`)
}

func (s *UserInfoCommandSuite) TestUserInfoWithUsername(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand(), "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `user-name: foobar
display-name: Foo Bar
date-created: 1981-02-27
last-connection: 2014-01-01
`)
}

func (*UserInfoCommandSuite) TestUserInfoUserDoesNotExist(c *gc.C) {
	_, err := testing.RunCommand(c, newUserInfoCommand(), "barfoo")
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (*UserInfoCommandSuite) TestUserInfoFormatJson(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
{"user-name":"user-test","display-name":"","date-created":"1981-02-27","last-connection":"2014-01-01"}
`[1:])
}

func (*UserInfoCommandSuite) TestUserInfoFormatJsonWithUsername(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand(), "foobar", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
{"user-name":"foobar","display-name":"Foo Bar","date-created":"1981-02-27","last-connection":"2014-01-01"}
`[1:])
}

func (*UserInfoCommandSuite) TestUserInfoFormatYaml(c *gc.C) {
	context, err := testing.RunCommand(c, newUserInfoCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `user-name: user-test
display-name: ""
date-created: 1981-02-27
last-connection: 2014-01-01
`)
}

func (*UserInfoCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserInfoCommand(), "username", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

type userFriendlyDurationSuite struct{}

var _ = gc.Suite(&userFriendlyDurationSuite{})

func (*userFriendlyDurationSuite) TestFormat(c *gc.C) {
	now := time.Now()
	for _, test := range []struct {
		other    time.Time
		expected string
	}{
		{
			other:    now,
			expected: "just now",
		}, {
			other:    now.Add(-1 * time.Second),
			expected: "just now",
		}, {
			other:    now.Add(-2 * time.Second),
			expected: "2 seconds ago",
		}, {
			other:    now.Add(-59 * time.Second),
			expected: "59 seconds ago",
		}, {
			other:    now.Add(-60 * time.Second),
			expected: "1 minute ago",
		}, {
			other:    now.Add(-61 * time.Second),
			expected: "1 minute ago",
		}, {
			other:    now.Add(-2 * time.Minute),
			expected: "2 minutes ago",
		}, {
			other:    now.Add(-59 * time.Minute),
			expected: "59 minutes ago",
		}, {
			other:    now.Add(-60 * time.Minute),
			expected: "1 hour ago",
		}, {
			other:    now.Add(-61 * time.Minute),
			expected: "1 hour ago",
		}, {
			other:    now.Add(-2 * time.Hour),
			expected: "2 hours ago",
		}, {
			other:    now.Add(-23 * time.Hour),
			expected: "23 hours ago",
		}, {
			other:    now.Add(-24 * time.Hour),
			expected: now.Add(-24 * time.Hour).Format("2006-01-02"),
		}, {
			other:    now.Add(-96 * time.Hour),
			expected: now.Add(-96 * time.Hour).Format("2006-01-02"),
		},
	} {
		obtained := user.UserFriendlyDuration(test.other, now)
		c.Check(obtained, gc.Equals, test.expected)
	}
}
