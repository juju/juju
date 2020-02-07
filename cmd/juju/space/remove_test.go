// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"bytes"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/space"
)

type RemoveSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&RemoveSuite{})

func (s *RemoveSuite) SetUpTest(c *gc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.newCommand = space.NewRemoveCommand
}

func (s *RemoveSuite) runCommand(c *gc.C, api space.SpaceAPI, name ...string) (*cmd.Context, *space.RemoveCommand, error) {
	base := space.NewSpaceCommandBase(api)
	command := space.RemoveCommand{
		SpaceCommandBase: base,
	}
	ctx, err := cmdtesting.RunCommand(c, &command, name...)
	return ctx, &command, err
}

func (s *RemoveSuite) TestInit(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	for i, test := range []struct {
		about      string
		args       []string
		expectName string
		expectErr  string
	}{{
		about:     "no arguments",
		expectErr: "space name is required",
	}, {
		about:     "invalid space name",
		args:      s.Strings("%inv$alid", "new-name"),
		expectErr: `"%inv\$alid" is not a valid space name`,
	}, {
		about:      "multiple space names aren't allowed",
		args:       s.Strings("a-space", "another-space"),
		expectErr:  `unrecognized args: \["another-space"\]`,
		expectName: "a-space",
	}, {
		about:      "delete a valid space name",
		args:       s.Strings("myspace"),
		expectName: "myspace",
	}} {
		c.Logf("test #%d: %s", i, test.about)
		if test.expectErr == "" {
			api.EXPECT().RemoveSpace(test.expectName).Return(nil)
		}
		_, cmd, err := s.runCommand(c, api, test.args...)
		if test.expectErr != "" {
			prefixedErr := "invalid arguments specified: " + test.expectErr
			c.Check(err, gc.ErrorMatches, prefixedErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(cmd.Name(), gc.Equals, test.expectName)
		}
	}
}

func (s *RemoveSuite) TestRunWithValidSpaceSucceeds(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	spaceName := "default"
	api.EXPECT().RemoveSpace(spaceName).Return(nil)
	ctx, _, err := s.runCommand(c, api, spaceName)

	c.Assert(err, gc.IsNil)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "removed space \"default\"\n")
}

func (s *RemoveSuite) TestRunWhenSpacesAPIFails(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	spaceName := "default"
	apiErr := &params.Error{Code: params.CodeOperationBlocked, Message: "nope"}
	api.EXPECT().RemoveSpace(spaceName).Return(apiErr)
	ctx, _, err := s.runCommand(c, api, spaceName)

	c.Assert(err, gc.ErrorMatches, "cannot remove space \"default\": nope")
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")

}
