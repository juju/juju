// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package environment_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/testing"
)

type UsersCommandSuite struct {
	fake *fakeEnvUsersClient
}

var _ = gc.Suite(&UsersCommandSuite{})

type fakeEnvUsersClient struct {
	users []params.EnvUserInfo
}

func (f *fakeEnvUsersClient) Close() error {
	return nil
}

func (f *fakeEnvUsersClient) EnvironmentUserInfo() ([]params.EnvUserInfo, error) {
	return f.users, nil
}

func (s *UsersCommandSuite) SetUpTest(c *gc.C) {
	last1 := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
	last2 := time.Date(2015, 3, 1, 0, 0, 0, 0, time.UTC)

	userlist := []params.EnvUserInfo{
		{
			UserName:       "admin@local",
			DisplayName:    "admin",
			CreatedBy:      "admin@local",
			DateCreated:    time.Date(2014, 7, 20, 9, 0, 0, 0, time.UTC),
			LastConnection: &last1,
		}, {
			UserName:       "bob@local",
			DisplayName:    "Bob",
			CreatedBy:      "admin@local",
			DateCreated:    time.Date(2015, 2, 15, 9, 0, 0, 0, time.UTC),
			LastConnection: &last2,
		}, {
			UserName:    "charlie@ubuntu.com",
			DisplayName: "Charlie",
			CreatedBy:   "admin@local",
			DateCreated: time.Date(2015, 2, 15, 9, 0, 0, 0, time.UTC),
		},
	}

	s.fake = &fakeEnvUsersClient{users: userlist}
}

func (s *UsersCommandSuite) TestEnvUsers(c *gc.C) {
	context, err := testing.RunCommand(c, environment.NewUsersCommand(s.fake))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME                DATE CREATED  LAST CONNECTION\n"+
		"admin@local         2014-07-20    2015-03-20\n"+
		"bob@local           2015-02-15    2015-03-01\n"+
		"charlie@ubuntu.com  2015-02-15    never connected\n"+
		"\n")
}

func (s *UsersCommandSuite) TestEnvUsersFormatJson(c *gc.C) {
	context, err := testing.RunCommand(c, environment.NewUsersCommand(s.fake), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "["+
		`{"user-name":"admin@local","date-created":"2014-07-20","last-connection":"2015-03-20"},`+
		`{"user-name":"bob@local","date-created":"2015-02-15","last-connection":"2015-03-01"},`+
		`{"user-name":"charlie@ubuntu.com","date-created":"2015-02-15","last-connection":"never connected"}`+
		"]\n")
}

func (s *UsersCommandSuite) TestUserInfoFormatYaml(c *gc.C) {
	context, err := testing.RunCommand(c, environment.NewUsersCommand(s.fake), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"- user-name: admin@local\n"+
		"  date-created: 2014-07-20\n"+
		"  last-connection: 2015-03-20\n"+
		"- user-name: bob@local\n"+
		"  date-created: 2015-02-15\n"+
		"  last-connection: 2015-03-01\n"+
		"- user-name: charlie@ubuntu.com\n"+
		"  date-created: 2015-02-15\n"+
		"  last-connection: never connected\n")
}

func (s *UsersCommandSuite) TestUnrecognizedArg(c *gc.C) {
	_, err := testing.RunCommand(c, environment.NewUsersCommand(s.fake), "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
