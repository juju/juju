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

type RemoveUserCommandSuite struct {
	BaseSuite
	mockAPI *mockRemoveUserAPI
}

var _ = gc.Suite(&RemoveUserCommandSuite{})

func (s *RemoveUserCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveUserAPI{}
}

type mockRemoveUserAPI struct {
	username string
}

func (*mockRemoveUserAPI) Close() error { return nil }

func (m *mockRemoveUserAPI) RemoveUser(username string) error {
	m.username = username
	return nil
}

func (s *RemoveUserCommandSuite) run(c *gc.C, name string) (*cmd.Context, error) {
	removeCommand, _ := user.NewRemoveCommandForTest(s.mockAPI, s.store)
	return testing.RunCommand(c, removeCommand, name)
}

func (s *RemoveUserCommandSuite) TestInit(c *gc.C) {
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
		wrappedCommand, command := user.NewRemoveCommandForTest(s.mockAPI, s.store)
		err := testing.InitCommand(wrappedCommand, test.args)
		c.Check(command.ConfirmDelete, jc.DeepEquals, test.confirm)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *RemoveUserCommandSuite) TestRemove(c *gc.C) {
	username := "testing"
	command, _ := user.NewRemoveCommandForTest(s.mockAPI, s.store)
	_, err := testing.RunCommand(c, command, "-y", username)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, username)

}

func (s *RemoveUserCommandSuite) TestRemovePrompts(c *gc.C) {
	username := "testing"
	expected := `
WARNING! This command will remove the user "testing" from the "testing" controller.

Continue (y/N)? `[1:]
	command, _ := user.NewRemoveCommandForTest(s.mockAPI, s.store)
	ctx, _ := testing.RunCommand(c, command, username)
	c.Assert(testing.Stdout(ctx), jc.DeepEquals, expected)

}
