// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"runtime"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
)

type varsSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&varsSuite{})

func (s *varsSuite) TestJujuXDGDataHomeEnvVar(c *tc.C) {
	path := "/foo/bar/baz"
	s.PatchEnvironment(osenv.JujuXDGDataHomeEnvKey, path)
	c.Assert(osenv.JujuXDGDataHomeDir(), tc.Equals, path)
}

func (s *varsSuite) TestBlankJujuXDGDataHomeEnvVar(c *tc.C) {
	s.PatchEnvironment(osenv.JujuXDGDataHomeEnvKey, "")

	if runtime.GOOS == "windows" {
		s.PatchEnvironment("APPDATA", `P:\foobar`)
	} else {
		s.PatchEnvironment("HOME", "/foobar")
	}
	c.Assert(osenv.JujuXDGDataHomeDir(), tc.Not(tc.Equals), "")

	if runtime.GOOS == "windows" {
		c.Assert(osenv.JujuXDGDataHomeDir(), tc.Equals, osenv.JujuXDGDataHomeWin())
	} else {
		c.Assert(osenv.JujuXDGDataHomeDir(), tc.Equals, osenv.JujuXDGDataHomeLinux())
	}
}

func (s *varsSuite) TestMergeEnvironment(c *tc.C) {
	c.Check(osenv.MergeEnvironment(nil, nil), tc.HasLen, 0)
	newValues := map[string]string{"a": "baz", "c": "omg"}
	created := osenv.MergeEnvironment(nil, newValues)
	expected := map[string]string{"a": "baz", "c": "omg"}
	c.Check(created, jc.DeepEquals, expected)
	// Show that the map returned isn't the one passed in.
	newValues["d"] = "another"
	c.Check(created, jc.DeepEquals, expected)
}

func (s *varsSuite) TestMergeEnvWin(c *tc.C) {
	initial := map[string]string{"a": "foo", "b": "bar", "foo": "val"}
	newValues := map[string]string{"a": "baz", "c": "omg", "FOO": "val2", "d": "another"}

	created := osenv.MergeEnvWin(initial, newValues)
	expected := map[string]string{"a": "baz", "b": "bar", "c": "omg", "foo": "val2", "d": "another"}
	// The returned value is the initial map.
	c.Check(created, jc.DeepEquals, expected)
	c.Check(initial, jc.DeepEquals, expected)
}

func (s *varsSuite) TestMergeEnvUnix(c *tc.C) {
	initial := map[string]string{"a": "foo", "b": "bar"}
	newValues := map[string]string{"a": "baz", "c": "omg", "d": "another"}

	created := osenv.MergeEnvUnix(initial, newValues)
	expected := map[string]string{"a": "baz", "b": "bar", "c": "omg", "d": "another"}
	// The returned value is the initial map.
	c.Check(created, jc.DeepEquals, expected)
	c.Check(initial, jc.DeepEquals, expected)
}
