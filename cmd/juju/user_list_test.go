// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package main

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/testing"
)

// All of the functionality of the UserInfo api call is contained elsewhere.
// This suite provides basic tests for the "user info" command
type UserListCommandSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&UserListCommandSuite{})

func (s *UserListCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.PatchValue(&getUserInfoAPI, func(*UserCommandBase) (UserInfoAPI, error) {
		return &fakeUserListAPI{}, nil
	})
}

func newUserListCommand() cmd.Command {
	return envcmd.Wrap(&UserListCommand{})
}

type fakeUserListAPI struct{}

func (*fakeUserListAPI) Close() error {
	return nil
}

func (f *fakeUserListAPI) UserInfo(tags []names.UserTag, includeDeactivated bool) ([]params.UserInfo, error) {
	if len(tags) > 0 {
		return nil, errors.Errorf("expected no tags, got %d", len(tags))
	}
	now := time.Now().UTC().Round(time.Second)
	last1 := time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC)
	// The extra two seconds here are needed to make sure
	// we don't get intermittent failures in formatting.
	last2 := now.Add(-35*time.Minute + -2*time.Second)
	result := []params.UserInfo{
		{
			Username:       "adam",
			DisplayName:    "Adam Zulu",
			DateCreated:    time.Date(2012, 10, 8, 0, 0, 0, 0, time.UTC),
			LastConnection: &last1,
		}, {
			Username:       "barbara",
			DisplayName:    "Barbara Yellow",
			DateCreated:    time.Date(2013, 5, 2, 0, 0, 0, 0, time.UTC),
			LastConnection: &now,
		}, {
			Username:    "charlie",
			DisplayName: "Charlie Xavier",
			// The extra two minutes here are needed to make sure
			// we don't get intermittent failures in formatting.
			DateCreated: now.Add(-6*time.Hour + -2*time.Minute),
		},
	}
	if includeDeactivated {
		result = append(result, params.UserInfo{
			Username:       "davey",
			DisplayName:    "Davey Willow",
			DateCreated:    time.Date(2014, 10, 9, 0, 0, 0, 0, time.UTC),
			LastConnection: &last2,
			Deactivated:    true,
		})
	}
	return result, nil
}

func (s *UserListCommandSuite) TestUserInfo(c *gc.C) {
	context, err := testing.RunCommand(c, newUserListCommand())
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME     DISPLAY NAME    DATE CREATED  LAST CONNECTION\n"+
		"adam     Adam Zulu       2012-10-08    2014-01-01\n"+
		"barbara  Barbara Yellow  2013-05-02    just now\n"+
		"charlie  Charlie Xavier  6 hours ago   not connected yet\n"+
		"\n")
}

func (s *UserListCommandSuite) TestUserInfoWithDisabled(c *gc.C) {
	context, err := testing.RunCommand(c, newUserListCommand(), "--show-disabled")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME     DISPLAY NAME    DATE CREATED  LAST CONNECTION\n"+
		"adam     Adam Zulu       2012-10-08    2014-01-01\n"+
		"barbara  Barbara Yellow  2013-05-02    just now\n"+
		"charlie  Charlie Xavier  6 hours ago   not connected yet\n"+
		"davey    Davey Willow    2014-10-09    35 minutes ago (disabled)\n"+
		"\n")
}

func (s *UserListCommandSuite) TestUserInfoExactTime(c *gc.C) {
	context, err := testing.RunCommand(c, newUserListCommand(), "--exact-time")
	c.Assert(err, gc.IsNil)
	dateRegex := `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \+0000 UTC`
	c.Assert(testing.Stdout(context), gc.Matches, ""+
		"NAME     DISPLAY NAME    DATE CREATED                   LAST CONNECTION\n"+
		"adam     Adam Zulu       2012-10-08 00:00:00 \\+0000 UTC  2014-01-01 00:00:00 \\+0000 UTC\n"+
		"barbara  Barbara Yellow  2013-05-02 00:00:00 \\+0000 UTC  "+dateRegex+"\n"+
		"charlie  Charlie Xavier  "+dateRegex+"  not connected yet\n"+
		"\n")
}

func (*UserListCommandSuite) TestUserInfoFormatJson(c *gc.C) {
	context, err := testing.RunCommand(c, newUserListCommand(), "--format", "json")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "["+
		`{"user-name":"adam","display-name":"Adam Zulu","date-created":"2012-10-08","last-connection":"2014-01-01"},`+
		`{"user-name":"barbara","display-name":"Barbara Yellow","date-created":"2013-05-02","last-connection":"just now"},`+
		`{"user-name":"charlie","display-name":"Charlie Xavier","date-created":"6 hours ago","last-connection":"not connected yet"}`+
		"]\n")
}

func (*UserListCommandSuite) TestUserInfoFormatYaml(c *gc.C) {
	context, err := testing.RunCommand(c, newUserListCommand(), "--format", "yaml")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"- user-name: adam\n"+
		"  display-name: Adam Zulu\n"+
		"  date-created: 2012-10-08\n"+
		"  last-connection: 2014-01-01\n"+
		"- user-name: barbara\n"+
		"  display-name: Barbara Yellow\n"+
		"  date-created: 2013-05-02\n"+
		"  last-connection: just now\n"+
		"- user-name: charlie\n"+
		"  display-name: Charlie Xavier\n"+
		"  date-created: 6 hours ago\n"+
		"  last-connection: not connected yet\n")
}

func (*UserListCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, newUserListCommand(), "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
