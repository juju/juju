// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	cmdenvironment "github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

type cmdEnvironmentSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&cmdEnvironmentSuite{})

func (s *cmdEnvironmentSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.JES)
}

func runShare(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdenvironment.ShareCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdEnvironmentSuite) TestEnvironmentShareCmdStack(c *gc.C) {
	username := "bar@ubuntuone"
	context := runShare(c, []string{username})
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	user := names.NewUserTag(username)
	envUser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserName(), gc.Equals, user.Username())
	c.Assert(envUser.CreatedBy(), gc.Equals, s.AdminUserTag(c).Username())
	c.Assert(envUser.LastConnection(), gc.IsNil)
}

func runUnshare(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdenvironment.UnshareCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdEnvironmentSuite) TestEnvironmentUnshareCmdStack(c *gc.C) {
	// Firstly share an environment with a user
	username := "bar@ubuntuone"
	context := runShare(c, []string{username})
	user := names.NewUserTag(username)
	envuser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envuser, gc.NotNil)

	// Then test that the unshare command stack is hooked up
	context = runUnshare(c, []string{username})
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	envuser, err = s.State.EnvironmentUser(user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(envuser, gc.IsNil)
}

func (s *cmdEnvironmentSuite) TestEnvironmentUsersCmd(c *gc.C) {
	// Firstly share an environment with a user
	username := "bar@ubuntuone"
	context := runShare(c, []string{username})
	user := names.NewUserTag(username)
	envuser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envuser, gc.NotNil)

	context, err = testing.RunCommand(c, envcmd.Wrap(&cmdenvironment.UsersCommand{}))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME               DATE CREATED  LAST CONNECTION\n"+
		"dummy-admin@local  just now      just now\n"+
		"bar@ubuntuone      just now      never connected\n"+
		"\n")

}
