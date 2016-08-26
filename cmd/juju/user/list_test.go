// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package user_test

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
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
}

var _ = gc.Suite(&UserListCommandSuite{})

func (s *UserListCommandSuite) newUserListCommand() cmd.Command {
	return user.NewListCommandForTest(&fakeUserListAPI{}, s.store)
}

type fakeUserListAPI struct{}

func (*fakeUserListAPI) Close() error {
	return nil
}

func (f *fakeUserListAPI) UserInfo(usernames []string, all usermanager.IncludeDisabled) ([]params.UserInfo, error) {
	if len(usernames) > 0 {
		return nil, errors.Errorf("expected no usernames, got %d", len(usernames))
	}
	// lp:1558657
	now := time.Now().UTC().Round(time.Second)
	last1 := time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC)
	// The extra two seconds here are needed to make sure
	// we don't get intermittent failures in formatting.
	last2 := now.Add(-35*time.Minute + -2*time.Second)
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
			// The extra two minutes here are needed to make sure
			// we don't get intermittent failures in formatting.
			DateCreated: now.Add(-6*time.Hour + -2*time.Minute),
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
	dateRegex := `\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \+0000 UTC`
	c.Assert(testing.Stdout(context), gc.Matches, ""+
		"CONTROLLER: testing\n\n"+
		"NAME     DISPLAY NAME    ACCESS     DATE CREATED                   LAST CONNECTION\n"+
		"adam\\*    Adam Zulu       login      2012-10-08 00:00:00 \\+0000 UTC  2014-01-01 00:00:00 \\+0000 UTC\n"+
		"barbara  Barbara Yellow  addmodel   2013-05-02 00:00:00 \\+0000 UTC  "+dateRegex+"\n"+
		"charlie  Charlie Xavier  superuser  "+dateRegex+"  never connected\n"+
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

func (s *UserListCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, s.newUserListCommand(), "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
