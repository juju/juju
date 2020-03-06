// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/cmd/juju/commands"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// UserSuite tests the connectivity of all the user subcommands. These tests
// go from the command line, api client, api server, db. The db changes are
// then checked.  Only one test for each command is done here to check
// connectivity.  Exhaustive tests are at each layer.
type UserSuite struct {
	jujutesting.JujuConnSuite
}

func (s *UserSuite) RunUserCommand(c *gc.C, stdin string, args ...string) (*cmd.Context, error) {
	context := cmdtesting.Context(c)
	if stdin != "" {
		context.Stdin = strings.NewReader(stdin)
	}
	jujuCmd := commands.NewJujuCommand(context, "")
	err := cmdtesting.InitCommand(jujuCmd, args)
	c.Assert(err, jc.ErrorIsNil)
	err = jujuCmd.Run(context)
	return context, err
}

func (s *UserSuite) TestUserAdd(c *gc.C) {
	ctx, err := s.RunUserCommand(c, "", "add-user", "test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.HasPrefix, `User "test" added`)
	user, err := s.State.User(names.NewLocalUserTag("test"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)
}

func (s *UserSuite) TestUserChangePassword(c *gc.C) {
	user, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsTrue)
	_, err = s.RunUserCommand(c, "not-dummy-secret\nnot-dummy-secret\n", "change-user-password")
	c.Assert(err, jc.ErrorIsNil)
	err = user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsFalse)
	c.Assert(user.PasswordValid("not-dummy-secret"), jc.IsTrue)
}

func (s *UserSuite) TestUserResetPasswordForSelf(c *gc.C) {
	user, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsTrue)

	// Should not be able to reset own password
	context, err := s.RunUserCommand(c, "", "change-user-password", "--reset")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), gc.Matches, `
You cannot reset your own password.
If you want to change it, please call `[1:]+"`juju change-user-password`"+` without --reset option.
`)
	err = user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsTrue)
}

func (s *UserSuite) TestUserResetPasswordForOther(c *gc.C) {
	username := "bob"
	context := run(c, nil, "add-user", username, "Bob Dobbs")
	c.Check(cmdtesting.Stderr(context), gc.Equals, "")

	context, err := s.RunUserCommand(c, "", "change-user-password", "--reset", username)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), gc.Matches, `
Password for "bob" has been reset.
Ask the user to run:
     juju register (.+)
`[1:])
}

func (s *UserSuite) TestUserResetPasswordFail(c *gc.C) {
	context, err := s.RunUserCommand(c, "", "change-user-password", "--reset", "bob")
	c.Assert(err, gc.ErrorMatches, "cmd: error out silently")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), gc.Matches, "ERROR permission denied\n")
}

func (s *UserSuite) TestUserInfo(c *gc.C) {
	user, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("dummy-secret"), jc.IsTrue)
	ctx, err := s.RunUserCommand(c, "", "show-user")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.Contains, "user-name: admin")
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

func (s *UserSuite) TestRemoveUserPrompt(c *gc.C) {
	expected := `
WARNING! This command will permanently archive the user "jjam" on the "kontroll"
controller.

This action is irreversible. If you wish to temporarily disable the
user please use the`[1:] + " `juju disable-user` " + `command. See
` + " `juju help disable-user` " + `for more details.

Continue (y/N)? `
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "jjam"})
	ctx, _ := s.RunUserCommand(c, "", "remove-user", "jjam")
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, expected)
}

func (s *UserSuite) TestRemoveUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "jjam"})
	_, err := s.RunUserCommand(c, "", "remove-user", "-y", "jjam")
	c.Assert(err, jc.ErrorIsNil)
	err = user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDeleted(), jc.IsTrue)
}

func (s *UserSuite) TestRemoveUserLongForm(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "jjam"})
	_, err := s.RunUserCommand(c, "", "remove-user", "--yes", "jjam")
	c.Assert(err, jc.ErrorIsNil)
	err = user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDeleted(), jc.IsTrue)
}

func (s *UserSuite) TestUserList(c *gc.C) {
	ctx, err := s.RunUserCommand(c, "", "list-users")
	c.Assert(err, jc.ErrorIsNil)
	periodPattern := `(just now|\d+ \S+ ago)`
	expected := fmt.Sprintf(`
Controller: kontroll

Name\s+Display name\s+Access\s+Date created\s+Last connection
admin.*\s+admin\s+superuser\s+%s\s+%s

`[1:], periodPattern, periodPattern)
	c.Assert(cmdtesting.Stdout(ctx), gc.Matches, expected)
}
