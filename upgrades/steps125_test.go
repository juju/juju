// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"errors"

	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
)

type steps125Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps125Suite{})

func (s *steps125Suite) TestStateStepsFor125(c *gc.C) {
	expected := []string{
		"set hosted environment count to number of hosted environments",
		"tag machine instances",
	}
	assertStateSteps(c, version.MustParse("1.25.0"), expected)
}

func (s *steps125Suite) TestStepsFor125(c *gc.C) {
	expected := []string{
		"remove Jujud.pass file on windows",
	}
	assertSteps(c, version.MustParse("1.25.0"), expected)
}

type mockOSRemove struct {
	called     bool
	path       string
	shouldFail bool
}

func (m *mockOSRemove) osRemove(path string) error {
	m.called = true
	m.path = path
	if m.shouldFail {
		return errors.New("i done error'd")
	}
	return nil
}

var removeFileTests = []struct {
	os           version.OSType
	callExpected bool
	shouldFail   bool
}{
	{
		os:           version.Ubuntu,
		callExpected: false,
		shouldFail:   false,
	},
	{
		os:           version.Windows,
		callExpected: true,
		shouldFail:   false,
	},
	{
		os:           version.Windows,
		callExpected: true,
		shouldFail:   true,
	},
}

func (s *steps125Suite) TestRemoveJujudPass(c *gc.C) {
	for _, t := range removeFileTests {
		mock := &mockOSRemove{shouldFail: t.shouldFail}
		s.PatchValue(upgrades.OsRemove, mock.osRemove)
		s.PatchValue(&version.Current.OS, t.os)
		err := upgrades.RemoveJujudpass(nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mock.called, gc.Equals, t.callExpected)
	}
}
