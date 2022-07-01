// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package user_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/api/client/usermanager"
	"github.com/juju/juju/v2/cmd/juju/user"
	"github.com/juju/juju/v2/jujuclient"
	"github.com/juju/juju/v2/rpc/params"
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

func (f *fakeUserListAPI) ModelUserInfo(modelUUID string) ([]params.ModelUserInfo, error) {
	last1 := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
	last2 := time.Date(2015, 3, 1, 0, 0, 0, 0, time.UTC)

	tag := names.NewModelTag(modelUUID).String()
	userlist := []params.ModelUserInfo{
		{
			ModelTag:       tag,
			UserName:       "admin",
			LastConnection: &last1,
			Access:         "write",
		}, {
			ModelTag:       tag,
			UserName:       "adam",
			DisplayName:    "Adam",
			LastConnection: &last2,
			Access:         "read",
		}, {
			ModelTag:    tag,
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
			Access:         "add-model",
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
		User:     "adam",
		Password: "password",
	}
}

func (s *UserListCommandSuite) TestUserInfo(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newUserListCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Controller: testing

Name     Display name    Access     Date created  Last connection
adam*    Adam Zulu       login      2012-10-08    2014-01-01
barbara  Barbara Yellow  add-model  2013-05-02    just now
charlie  Charlie Xavier  superuser  6 hours ago   never connected

`[1:])
}

func (s *UserListCommandSuite) TestUserInfoWithDisabled(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newUserListCommand(), "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Controller: testing

Name     Display name    Access     Date created  Last connection
adam*    Adam Zulu       login      2012-10-08    2014-01-01
barbara  Barbara Yellow  add-model  2013-05-02    just now
charlie  Charlie Xavier  superuser  6 hours ago   never connected
davey    Davey Willow               2014-10-09    35 minutes ago (disabled)

`[1:])
}

func (s *UserListCommandSuite) TestUserInfoExactTime(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newUserListCommand(), "--exact-time")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Controller: testing

Name     Display name    Access     Date created                   Last connection
adam*    Adam Zulu       login      2012-10-08 00:00:00 +0000 UTC  2014-01-01 00:00:00 +0000 UTC
barbara  Barbara Yellow  add-model  2013-05-02 00:00:00 +0000 UTC  2016-09-15 12:00:00 +0000 UTC
charlie  Charlie Xavier  superuser  2016-09-15 06:00:00 +0000 UTC  never connected

`[1:])
}

func (s *UserListCommandSuite) TestUserInfoFormatJson(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newUserListCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "["+
		`{"user-name":"adam","display-name":"Adam Zulu","access":"login","date-created":"2012-10-08","last-connection":"2014-01-01"},`+
		`{"user-name":"barbara","display-name":"Barbara Yellow","access":"add-model","date-created":"2013-05-02","last-connection":"just now"},`+
		`{"user-name":"charlie","display-name":"Charlie Xavier","access":"superuser","date-created":"6 hours ago","last-connection":"never connected"}`+
		"]\n")
}

func (s *UserListCommandSuite) TestUserInfoFormatYaml(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newUserListCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
- user-name: adam
  display-name: Adam Zulu
  access: login
  date-created: "2012-10-08"
  last-connection: "2014-01-01"
- user-name: barbara
  display-name: Barbara Yellow
  access: add-model
  date-created: "2013-05-02"
  last-connection: just now
- user-name: charlie
  display-name: Charlie Xavier
  access: superuser
  date-created: 6 hours ago
  last-connection: never connected
`[1:])
}

func (s *UserListCommandSuite) TestModelUsers(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newUserListCommand(), "test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Name                Display name  Access  Last connection
adam*               Adam          read    2015-03-01
admin                             write   2015-03-20
charlie@ubuntu.com  Charlie       read    never connected

`[1:])
}

func (s *UserListCommandSuite) TestModelUsersFormatJson(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newUserListCommand(), "test", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "{"+
		`"adam":{"display-name":"Adam","access":"read","last-connection":"2015-03-01"},`+
		`"admin":{"access":"write","last-connection":"2015-03-20"},`+
		`"charlie@ubuntu.com":{"display-name":"Charlie","access":"read","last-connection":"never connected"}`+
		"}\n")
}

func (s *UserListCommandSuite) TestModelUsersInfoFormatYaml(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newUserListCommand(), "test", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
adam:
  display-name: Adam
  access: read
  last-connection: "2015-03-01"
admin:
  access: write
  last-connection: "2015-03-20"
charlie@ubuntu.com:
  display-name: Charlie
  access: read
  last-connection: never connected
`[1:])
}

func (s *UserListCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.newUserListCommand(), "model", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
