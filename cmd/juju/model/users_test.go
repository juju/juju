// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type UsersCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake  *fakeModelUsersClient
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&UsersCommandSuite{})

type fakeModelUsersClient struct {
	users []params.ModelUserInfo
}

func (f *fakeModelUsersClient) Close() error {
	return nil
}

func (f *fakeModelUsersClient) ModelUserInfo() ([]params.ModelUserInfo, error) {
	return f.users, nil
}

func (s *UsersCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	last1 := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
	last2 := time.Date(2015, 3, 1, 0, 0, 0, 0, time.UTC)

	userlist := []params.ModelUserInfo{
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

	s.fake = &fakeModelUsersClient{users: userlist}

	err := modelcmd.WriteCurrentController("testing")
	c.Assert(err, jc.ErrorIsNil)
	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = &jujuclient.ControllerAccounts{
		CurrentAccount: "admin@local",
	}
}

func (s *UsersCommandSuite) TestModelUsers(c *gc.C) {
	context, err := testing.RunCommand(c, model.NewUsersCommandForTest(s.fake, s.store), "-m", "dummymodel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME                DATE CREATED  LAST CONNECTION\n"+
		"admin@local         2014-07-20    2015-03-20\n"+
		"bob@local           2015-02-15    2015-03-01\n"+
		"charlie@ubuntu.com  2015-02-15    never connected\n"+
		"\n")
}

func (s *UsersCommandSuite) TestModelUsersFormatJson(c *gc.C) {
	context, err := testing.RunCommand(c, model.NewUsersCommandForTest(s.fake, s.store), "-m", "dummymodel", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "["+
		`{"user-name":"admin@local","date-created":"2014-07-20","last-connection":"2015-03-20"},`+
		`{"user-name":"bob@local","date-created":"2015-02-15","last-connection":"2015-03-01"},`+
		`{"user-name":"charlie@ubuntu.com","date-created":"2015-02-15","last-connection":"never connected"}`+
		"]\n")
}

func (s *UsersCommandSuite) TestUserInfoFormatYaml(c *gc.C) {
	context, err := testing.RunCommand(c, model.NewUsersCommandForTest(s.fake, s.store), "-m", "dummymodel", "--format", "yaml")
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
	_, err := testing.RunCommand(c, model.NewUsersCommandForTest(s.fake, s.store), "-m", "dummymodel", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
