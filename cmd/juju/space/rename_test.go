// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/space"
)

type RenameSuite struct {
	BaseSpaceSuite
}

func TestRenameSuite(t *stdtesting.T) { tc.Run(t, &RenameSuite{}) }
func (s *RenameSuite) SetUpTest(c *tc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.newCommand = space.NewRenameCommand
}

func (s *RenameSuite) TestInit(c *tc.C) {
	for i, test := range []struct {
		about         string
		args          []string
		expectName    string
		expectNewName string
		expectErr     string
	}{{
		about:     "no arguments",
		expectErr: "old-name is required",
	}, {
		about:     "no new name",
		args:      s.Strings("a-space"),
		expectErr: "new-name is required",
	}, {
		about:     "invalid space name - with invalid characters",
		args:      s.Strings("%inv$alid", "new-name"),
		expectErr: `"%inv\$alid" is not a valid space name`,
	}, {
		about:     "invalid space name - using underscores",
		args:      s.Strings("42_space", "new-name"),
		expectErr: `"42_space" is not a valid space name`,
	}, {
		about:     "valid space name with invalid new name",
		args:      s.Strings("a-space", "inv#alid"),
		expectErr: `"inv#alid" is not a valid space name`,
	}, {
		about:     "valid space name with CIDR as new name",
		args:      s.Strings("a-space", "1.2.3.4/24"),
		expectErr: `"1.2.3.4/24" is not a valid space name`,
	}, {
		about:         "more than two arguments",
		args:          s.Strings("a-space", "another-space", "rubbish"),
		expectErr:     `unrecognized args: \["rubbish"\]`,
		expectName:    "a-space",
		expectNewName: "another-space",
	}, {
		about:         "old and new names are the same",
		args:          s.Strings("a-space", "a-space"),
		expectName:    "a-space",
		expectNewName: "a-space",
		expectErr:     "old-name and new-name are the same",
	}, {
		about:         "all ok",
		args:          s.Strings("a-space", "another-space"),
		expectName:    "a-space",
		expectNewName: "another-space",
	}} {
		c.Logf("test #%d: %s", i, test.about)
		command, err := s.InitCommand(c, test.args...)
		if test.expectErr != "" {
			prefixedErr := "invalid arguments specified: " + test.expectErr
			c.Check(err, tc.ErrorMatches, prefixedErr)
		} else {
			c.Check(err, tc.ErrorIsNil)
			command := command.(*space.RenameCommand)
			c.Check(command.Name, tc.Equals, test.expectName)
			c.Check(command.NewName, tc.Equals, test.expectNewName)
		}
		// No API calls should be recorded at this stage.
		s.api.CheckCallNames(c)
	}
}

func (s *RenameSuite) TestRunWithValidNamesSucceeds(c *tc.C) {
	s.AssertRunSucceeds(c,
		`renamed space "a-space" to "another-space"\n`,
		"", // no stdout, just stderr
		"a-space", "another-space",
	)

	s.api.CheckCallNames(c, "RenameSpace", "Close")
	s.api.CheckCall(c, 0, "RenameSpace", "a-space", "another-space")
}

func (s *RenameSuite) TestRunWhenSpacesAPIFails(c *tc.C) {
	s.api.SetErrors(errors.New("boom"))

	_ = s.AssertRunFails(c,
		`cannot rename space "foo": boom`,
		"foo", "bar",
	)

	s.api.CheckCallNames(c, "RenameSpace", "Close")
	s.api.CheckCall(c, 0, "RenameSpace", "foo", "bar")
}
