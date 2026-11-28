// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"context"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/user"
)

type RemoveUserCommandSuite struct {
	BaseSuite
	mockAPI *mockRemoveUserAPI
}

func TestRemoveUserCommandSuite(t *testing.T) {
	tc.Run(t, &RemoveUserCommandSuite{})
}

func (s *RemoveUserCommandSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveUserAPI{}
}

type mockRemoveUserAPI struct {
	username string
}

func (*mockRemoveUserAPI) Close() error { return nil }

func (m *mockRemoveUserAPI) RemoveUser(ctx context.Context, username string) error {
	m.username = username
	return nil
}

func (s *RemoveUserCommandSuite) TestInit(c *tc.C) {
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
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		c.Check(command.ConfirmDelete, tc.DeepEquals, test.confirm)
		if test.errorString == "" {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorMatches, test.errorString)
		}
	}
}

func (s *RemoveUserCommandSuite) TestRemove(c *tc.C) {
	username := "testing"
	command, _ := user.NewRemoveCommandForTest(s.mockAPI, s.store)
	_, err := cmdtesting.RunCommand(c, command, "-y", username)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.mockAPI.username, tc.Equals, username)

}

func (s *RemoveUserCommandSuite) TestRemovePrompts(c *tc.C) {
	username := "alex"
	command, _ := user.NewRemoveCommandForTest(s.mockAPI, s.store)
	ctx, err := cmdtesting.RunCommand(c, command, username)
	c.Assert(err, tc.ErrorMatches, "user deletion aborted")
	c.Check(cmdtesting.Stdout(ctx), tc.Matches, `(?s).*"alex".*"testing".*Continue \(y/N\)\? `)
	c.Check(s.mockAPI.username, tc.Equals, "")
}
