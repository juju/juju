// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package user_test

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

// All of the functionality of the UserInfo api call is contained elsewhere.
// This suite provides basic tests for the "show-user" command
type UserListCommandSuite struct {
	BaseSuite

	clock fakeClock
}

var _ = gc.Suite(&UserListCommandSuite{})

func (s *UserListCommandSuite) newUserListCommand() cmd.Command {
	clock := &fakeClock{now: time.Date(2016, 9, 15, 12, 0, 0, 0, time.UTC)}
	api := &fakeUserListAPI{clock}
	return user.NewListCommandForTest(api, api, s.store, clock)
}

type fakeUserListAPI struct {
	clock *fakeClock
}

func (*fakeUserListAPI) Close() error {
	return nil
}

type fakeClock struct {
	clock.Clock
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeUserListAPI) ModelUserInfo() ([]params.ModelUserInfo, error) {
	last1 := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
	last2 := time.Date(2015, 3, 1, 0, 0, 0, 0, time.UTC)

	userlist := []params.ModelUserInfo{
		{
			UserName:       "admin@local",
			LastConnection: &last1,
			Access:         "write",
		}, {
			UserName:       "adam@local",
			DisplayName:    "Adam",
			LastConnection: &last2,
			Access:         "read",
		}, {
			UserName:    "charlie@ubuntu.com",
			DisplayName: "Charlie",
			Access:      "read",
		},
	}
	return userlist, nil
}

func (f *fakeUserListAPI) UserInfo(usernames []string, all usermanager.IncludeDisabled) ([]params.UserInfo, error) {
	if len(usernames) > 0 {
		return nil, errors.Errorf("expected no usernames, got %d", len(usernames))
	}
	now := f.clock.Now()
	last1 := time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC)
	last2 := now.Add(-35 * time.Minute)
	result := []params.UserInfo{
		{
			Username:       "adam",
			DisplayName:    "Adam Zulu",
			Access:         "login",
			DateCreated:    time.Date(2012, 10, 8, 0, 0, 0, 0, time.UTC),
			LastConnection: &last1,
		}, {
			Username:       "barbara",
			DisplayName:    "Barbara Yellow",
			Access:         "addmodel",
			DateCreated:    time.Date(2013, 5, 2, 0, 0, 0, 0, time.UTC),
			LastConnection: &now,
		}, {
			Username:    "charlie",
			DisplayName: "Charlie Xavier",
			Access:      "superuser",
			DateCreated: now.Add(-6 * time.Hour),
		},
	}
	if all {
		result = append(result, params.UserInfo{
			Username:       "davey",
			DisplayName:    "Davey Willow",
			DateCreated:    time.Date(2014, 10, 9, 0, 0, 0, 0, time.UTC),
			LastConnection: &last2,
			Disabled:       true,
		})
	}
	return result, nil
}

func (s *UserListCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User:     "adam@local",
		Password: "password",
	}
}

func (s *UserListCommandSuite) TestUserInfo(c *gc.C) {
	context, err := testing.RunCommand(c, s.newUserListCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"CONTROLLER: testing\n\n"+
		"NAME     DISPLAY NAME    ACCESS     DATE CREATED  LAST CONNECTION\n"+
		"adam*    Adam Zulu       login      2012-10-08    2014-01-01\n"+
		"barbara  Barbara Yellow  addmodel   2013-05-02    just now\n"+
		"charlie  Charlie Xavier  superuser  6 hours ago   never connected\n"+
		"\n")
}

func (s *UserListCommandSuite) TestUserInfoWithDisabled(c *gc.C) {
	context, err := testing.RunCommand(c, s.newUserListCommand(), "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"CONTROLLER: testing\n\n"+
		"NAME     DISPLAY NAME    ACCESS     DATE CREATED  LAST CONNECTION\n"+
		"adam*    Adam Zulu       login      2012-10-08    2014-01-01\n"+
		"barbara  Barbara Yellow  addmodel   2013-05-02    just now\n"+
		"charlie  Charlie Xavier  superuser  6 hours ago   never connected\n"+
		"davey    Davey Willow               2014-10-09    35 minutes ago (disabled)\n"+
		"\n")
}

func (s *UserListCommandSuite) TestUserInfoExactTime(c *gc.C) {
	context, err := testing.RunCommand(c, s.newUserListCommand(), "--exact-time")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"CONTROLLER: testing\n\n"+
		"NAME     DISPLAY NAME    ACCESS     DATE CREATED                   LAST CONNECTION\n"+
		"adam*    Adam Zulu       login      2012-10-08 00:00:00 +0000 UTC  2014-01-01 00:00:00 +0000 UTC\n"+
		"barbara  Barbara Yellow  addmodel   2013-05-02 00:00:00 +0000 UTC  2016-09-15 12:00:00 +0000 UTC\n"+
		"charlie  Charlie Xavier  superuser  2016-09-15 06:00:00 +0000 UTC  never connected\n"+
		"\n")
}

func (s *UserListCommandSuite) TestUserInfoFormatJson(c *gc.C) {
	context, err := testing.RunCommand(c, s.newUserListCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "["+
		`{"user-name":"adam","display-name":"Adam Zulu","access":"login","date-created":"2012-10-08","last-connection":"2014-01-01"},`+
		`{"user-name":"barbara","display-name":"Barbara Yellow","access":"addmodel","date-created":"2013-05-02","last-connection":"just now"},`+
		`{"user-name":"charlie","display-name":"Charlie Xavier","access":"superuser","date-created":"6 hours ago","last-connection":"never connected"}`+
		"]\n")
}

func (s *UserListCommandSuite) TestUserInfoFormatYaml(c *gc.C) {
	context, err := testing.RunCommand(c, s.newUserListCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"- user-name: adam\n"+
		"  display-name: Adam Zulu\n"+
		"  access: login\n"+
		"  date-created: 2012-10-08\n"+
		"  last-connection: 2014-01-01\n"+
		"- user-name: barbara\n"+
		"  display-name: Barbara Yellow\n"+
		"  access: addmodel\n"+
		"  date-created: 2013-05-02\n"+
		"  last-connection: just now\n"+
		"- user-name: charlie\n"+
		"  display-name: Charlie Xavier\n"+
		"  access: superuser\n"+
		"  date-created: 6 hours ago\n"+
		"  last-connection: never connected\n")
}

func (s *UserListCommandSuite) TestModelUsers(c *gc.C) {
	context, err := testing.RunCommand(c, s.newUserListCommand(), "admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME                DISPLAY NAME  ACCESS  LAST CONNECTION\n"+
		"adam@local*         Adam          read    2015-03-01\n"+
		"admin@local                       write   2015-03-20\n"+
		"charlie@ubuntu.com  Charlie       read    never connected\n"+
		"\n")
}

func (s *UserListCommandSuite) TestModelUsersFormatJson(c *gc.C) {
	context, err := testing.RunCommand(c, s.newUserListCommand(), "admin", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "{"+
		`"adam@local":{"display-name":"Adam","access":"read","last-connection":"2015-03-01"},`+
		`"admin@local":{"access":"write","last-connection":"2015-03-20"},`+
		`"charlie@ubuntu.com":{"display-name":"Charlie","access":"read","last-connection":"never connected"}`+
		"}\n")
}

func (s *UserListCommandSuite) TestModelUsersInfoFormatYaml(c *gc.C) {
	context, err := testing.RunCommand(c, s.newUserListCommand(), "admin", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"adam@local:\n"+
		"  display-name: Adam\n"+
		"  access: read\n"+
		"  last-connection: 2015-03-01\n"+
		"admin@local:\n"+
		"  access: write\n"+
		"  last-connection: 2015-03-20\n"+
		"charlie@ubuntu.com:\n"+
		"  display-name: Charlie\n"+
		"  access: read\n"+
		"  last-connection: never connected\n")
}

func (s *UserListCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, s.newUserListCommand(), "model", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
