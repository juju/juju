// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/testing"
)

type RemoveUserSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&RemoveUserSuite{})

func (s *RemoveUserSuite) TestRemoveUser(c *gc.C) {
	mock := &mockRemoveUserAPI{}
	s.PatchValue(&getRemoveUserAPI, func(*RemoveUserCommand) (removeUserAPI, error) {
		return mock, nil
	})

	users := []string{"foo", "bar", "baz"}
	for _, user := range users {
		_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveUserCommand{}), user)
		c.Assert(err, gc.IsNil)
	}
	c.Assert(mock.args, gc.DeepEquals, users)
}

func (s *RemoveUserSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveUserCommand{}), "foobar", "password")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["password"\]`)
}

func (s *RemoveUserSuite) TestNotEnoughArgs(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveUserCommand{}))
	c.Assert(err, gc.ErrorMatches, `no username supplied`)
}

type mockRemoveUserAPI struct {
	args []string
}

func (m *mockRemoveUserAPI) Close() error {
	return nil
}

func (m *mockRemoveUserAPI) RemoveUser(user string) error {
	m.args = append(m.args, user)
	return nil
}
