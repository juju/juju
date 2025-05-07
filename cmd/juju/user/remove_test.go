// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type RemoveUserCommandSuite struct {
	BaseSuite
	mockAPI *mockRemoveUserAPI
}

var _ = tc.Suite(&RemoveUserCommandSuite{})

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
		c.Check(command.ConfirmDelete, jc.DeepEquals, test.confirm)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorMatches, test.errorString)
		}
	}
}

func (s *RemoveUserCommandSuite) TestRemove(c *tc.C) {
	username := "testing"
	command, _ := user.NewRemoveCommandForTest(s.mockAPI, s.store)
	_, err := cmdtesting.RunCommand(c, command, "-y", username)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, tc.Equals, username)

}

func (s *RemoveUserCommandSuite) TestRemovePrompts(c *tc.C) {
	username := "testing"
	expected := `WARNING! This command will permanently archive the user "testing" on the "testing"
controller. This action is irreversible and you WILL NOT be able to reuse
username "testing".

If you wish to temporarily disable the user please use the` + " `juju disable-user`\n" + `command. See
` + " `juju help disable-user` " + `for more details.

Continue (y/N)? `
	command, _ := user.NewRemoveCommandForTest(s.mockAPI, s.store)
	ctx, _ := cmdtesting.RunCommand(c, command, username)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, expected)

}
