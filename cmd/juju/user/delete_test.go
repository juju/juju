// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package user_test

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type DeleteUserCommandSuite struct {
	BaseSuite
	mockAPI *mockDeleteUserAPI
}

var _ = gc.Suite(&DeleteUserCommandSuite{})

func (s *DeleteUserCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockDeleteUserAPI{}
}

type mockDeleteUserAPI struct {
	username string
}

func (*mockDeleteUserAPI) Close() error { return nil }

func (m *mockDeleteUserAPI) DeleteUser(username string) error {

	m.username = username
	return nil
}

func (s *DeleteUserCommandSuite) run(c *gc.C, name string) (*cmd.Context, error) {
	deleteCommand, _ := user.NewDeleteCommandForTest(s.mockAPI, s.store)
	return testing.RunCommand(c, deleteCommand, name)
}

func (s *DeleteUserCommandSuite) TestInit(c *gc.C) {
	table := []struct {
		args        []string
		confirm     bool
		errorString string
	}{{
		confirm:     false,
		errorString: "no username supplied",
	}, {
		args:        []string{"--yes"},
		confirm:     true,
		errorString: "no username supplied",
	}, {
		args:    []string{"--yes", "jjam"},
		confirm: true,
	}}
	for _, test := range table {
		wrappedCommand, command := user.NewDeleteCommandForTest(s.mockAPI, s.store)
		err := testing.InitCommand(wrappedCommand, test.args)
		c.Check(command.ConfirmDelete, jc.DeepEquals, test.confirm)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *DeleteUserCommandSuite) TestDelete(c *gc.C) {
	username := "testing"
	command, _ := user.NewDeleteCommandForTest(s.mockAPI, s.store)
	_, err := testing.RunCommand(c, command, username)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, username)

}
