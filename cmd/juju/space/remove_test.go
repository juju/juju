// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/feature"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
	coretesting "github.com/juju/juju/testing"
)

type RemoveSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&RemoveSuite{})

func (s *RemoveSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetFeatureFlags(feature.PostNetCLIMVP)
	s.BaseSpaceSuite.SetUpTest(c)
	s.command = space.NewRemoveCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *RemoveSuite) TestInit(c *gc.C) {
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
		// Create a new instance of the subcommand for each test, but
		// since we're not running the command no need to use
		// envcmd.Wrap().
		command := space.NewRemoveCommand(s.api)
		err := coretesting.InitCommand(command, test.args)
		if test.expectErr != "" {
			prefixedErr := "invalid arguments specified: " + test.expectErr
			c.Check(err, gc.ErrorMatches, prefixedErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(command.Name, gc.Equals, test.expectName)
		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *RemoveSuite) TestRunWithValidSpaceSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`removed space "myspace"\n`,
		"", // no stdout, just stderr
		"myspace",
	)

	s.api.CheckCallNames(c, "RemoveSpace", "Close")
	s.api.CheckCall(c, 0, "RemoveSpace", "myspace")
}

func (s *RemoveSuite) TestRunWhenSpacesAPIFails(c *gc.C) {
	s.api.SetErrors(errors.New("boom"))

	s.AssertRunFails(c,
		`cannot remove space "myspace": boom`,
		"myspace",
	)

	s.api.CheckCallNames(c, "RemoveSpace", "Close")
	s.api.CheckCall(c, 0, "RemoveSpace", "myspace")
}

func (s *RemoveSuite) TestRunAPIConnectFails(c *gc.C) {
	s.command = space.NewRemoveCommand(nil)
	s.AssertRunFails(c,
		"cannot connect to the API server: no environment specified",
		"myname", // Drop the args once RunWitnAPI is called internally.
	)
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
