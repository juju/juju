// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/model"
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
			LastConnection: &last1,
			Access:         "write",
		}, {
			UserName:       "bob@local",
			DisplayName:    "Bob",
			LastConnection: &last2,
			Access:         "read",
		}, {
			UserName:    "charlie@ubuntu.com",
			DisplayName: "Charlie",
			Access:      "read",
		},
	}

	s.fake = &fakeModelUsersClient{users: userlist}

	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = &jujuclient.ControllerAccounts{
		CurrentAccount: "admin@local",
	}
}

func (s *UsersCommandSuite) TestModelUsers(c *gc.C) {
	context, err := testing.RunCommand(c, model.NewUsersCommandForTest(s.fake, s.store), "-m", "admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME                          ACCESS  LAST CONNECTION\n"+
		"admin@local                   write   2015-03-20\n"+
		"bob@local (Bob)               read    2015-03-01\n"+
		"charlie@ubuntu.com (Charlie)  read    never connected\n"+
		"\n")
}

func (s *UsersCommandSuite) TestModelUsersFormatJson(c *gc.C) {
	context, err := testing.RunCommand(c, model.NewUsersCommandForTest(s.fake, s.store), "-m", "admin", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "{"+
		`"admin@local":{"access":"write","last-connection":"2015-03-20"},`+
		`"bob@local":{"display-name":"Bob","access":"read","last-connection":"2015-03-01"},`+
		`"charlie@ubuntu.com":{"display-name":"Charlie","access":"read","last-connection":"never connected"}`+
		"}\n")
}

func (s *UsersCommandSuite) TestUserInfoFormatYaml(c *gc.C) {
	context, err := testing.RunCommand(c, model.NewUsersCommandForTest(s.fake, s.store), "-m", "admin", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"admin@local:\n"+
		"  access: write\n"+
		"  last-connection: 2015-03-20\n"+
		"bob@local:\n"+
		"  display-name: Bob\n"+
		"  access: read\n"+
		"  last-connection: 2015-03-01\n"+
		"charlie@ubuntu.com:\n"+
		"  display-name: Charlie\n"+
		"  access: read\n"+
		"  last-connection: never connected\n")
}

func (s *UsersCommandSuite) TestUnrecognizedArg(c *gc.C) {
	_, err := testing.RunCommand(c, model.NewUsersCommandForTest(s.fake, s.store), "-m", "admin", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
