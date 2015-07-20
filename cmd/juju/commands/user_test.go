// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// UserSuite tests the connectivity of all the user subcommands. These tests
// go from the command line, api client, api server, db. The db changes are
// then checked.  Only one test for each command is done here to check
// connectivity.  Exhaustive tests are at each layer.
type UserSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&UserSuite{})

func (s *UserSuite) RunUserCommand(c *gc.C, commands ...string) (*cmd.Context, error) {
	args := []string{"user"}
	args = append(args, commands...)
	context := testing.Context(c)
	juju := NewJujuCommand(context)
	if err := testing.InitCommand(juju, args); err != nil {
		return context, err
	}
	return context, juju.Run(context)
}

func (s *UserSuite) TestUserAdd(c *gc.C) {
	ctx, err := s.RunUserCommand(c, "add", "test", "--generate")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), jc.HasPrefix, `user "test" added`)
	user, err := s.State.User(names.NewLocalUserTag("test"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)
}

func (s *UserSuite) TestUserChangePassword(c *gc.C) {
	user, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsTrue)
	_, err = s.RunUserCommand(c, "change-password", "--generate")
	c.Assert(err, jc.ErrorIsNil)
	user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsFalse)
}

func (s *UserSuite) TestUserInfo(c *gc.C) {
	user, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsTrue)
	ctx, err := s.RunUserCommand(c, "info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), jc.Contains, "user-name: dummy-admin")
}

func (s *UserSuite) TestUserDisable(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "barbara"})
	_, err := s.RunUserCommand(c, "disable", "barbara")
	c.Assert(err, jc.ErrorIsNil)
	user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsTrue)
}

func (s *UserSuite) TestUserEnable(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "barbara", Disabled: true})
	_, err := s.RunUserCommand(c, "enable", "barbara")
	c.Assert(err, jc.ErrorIsNil)
	user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)
}

func (s *UserSuite) TestUserList(c *gc.C) {
	ctx, err := s.RunUserCommand(c, "list")
	c.Assert(err, jc.ErrorIsNil)
	periodPattern := `(just now|\d+ \S+ ago)`
	expected := fmt.Sprintf(`
NAME\s+DISPLAY NAME\s+DATE CREATED\s+LAST CONNECTION
dummy-admin\s+dummy-admin\s+%s\s+%s

`[1:], periodPattern, periodPattern)
	c.Assert(testing.Stdout(ctx), gc.Matches, expected)
}
