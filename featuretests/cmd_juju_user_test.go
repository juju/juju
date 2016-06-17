// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/commands"
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

func (s *UserSuite) RunUserCommand(c *gc.C, stdin string, args ...string) (*cmd.Context, error) {
	context := testing.Context(c)
	if stdin != "" {
		context.Stdin = strings.NewReader(stdin)
	}
	jujuCmd := commands.NewJujuCommand(context)
	err := testing.InitCommand(jujuCmd, args)
	c.Assert(err, jc.ErrorIsNil)
	err = jujuCmd.Run(context)
	return context, err
}

func (s *UserSuite) TestUserAdd(c *gc.C) {
	ctx, err := s.RunUserCommand(c, "", "add-user", "test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), jc.HasPrefix, `User "test" added`)
	user, err := s.State.User(names.NewLocalUserTag("test"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)
}

func (s *UserSuite) TestUserAddGrantModel(c *gc.C) {
	sharedModelState := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "amodel",
	})
	defer sharedModelState.Close()

	ctx, err := s.RunUserCommand(c, "", "add-user", "test", "--models", "amodel")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), jc.HasPrefix, `User "test" added`)
	user, err := s.State.User(names.NewLocalUserTag("test"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)

	// Check model is shared with expected users.
	sharedModel, err := sharedModelState.Model()
	c.Assert(err, jc.ErrorIsNil)
	users, err := sharedModel.Users()
	c.Assert(err, jc.ErrorIsNil)
	var modelUserTags = make([]names.UserTag, len(users))
	for i, u := range users {
		modelUserTags[i] = u.UserTag()
	}
	c.Assert(modelUserTags, jc.SameContents, []names.UserTag{
		user.Tag().(names.UserTag),
		names.NewLocalUserTag("admin"),
	})
}

func (s *UserSuite) TestUserChangePassword(c *gc.C) {
	user, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsTrue)
	_, err = s.RunUserCommand(c, "not-dummy-secret\nnot-dummy-secret\n", "change-user-password")
	c.Assert(err, jc.ErrorIsNil)
	user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsFalse)
	c.Assert(user.PasswordValid("not-dummy-secret"), jc.IsTrue)
}

func (s *UserSuite) TestUserInfo(c *gc.C) {
	user, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsTrue)
	ctx, err := s.RunUserCommand(c, "", "show-user")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), jc.Contains, "user-name: admin")
}

func (s *UserSuite) TestUserDisable(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "barbara"})
	_, err := s.RunUserCommand(c, "", "disable-user", "barbara")
	c.Assert(err, jc.ErrorIsNil)
	user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsTrue)
}

func (s *UserSuite) TestUserEnable(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "barbara", Disabled: true})
	_, err := s.RunUserCommand(c, "", "enable-user", "barbara")
	c.Assert(err, jc.ErrorIsNil)
	user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)
}

func (s *UserSuite) TestUserList(c *gc.C) {
	ctx, err := s.RunUserCommand(c, "", "list-users")
	c.Assert(err, jc.ErrorIsNil)
	periodPattern := `(just now|\d+ \S+ ago)`
	expected := fmt.Sprintf(`
NAME\s+DISPLAY NAME\s+DATE CREATED\s+LAST CONNECTION
admin\s+admin\s+%s\s+%s

`[1:], periodPattern, periodPattern)
	c.Assert(testing.Stdout(ctx), gc.Matches, expected)
}
