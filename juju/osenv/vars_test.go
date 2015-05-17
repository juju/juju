// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"runtime"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type varsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&varsSuite{})

func (s *varsSuite) TestJujuHomeEnvVar(c *gc.C) {
	path := "/foo/bar/baz"
	s.PatchEnvironment(osenv.JujuHomeEnvKey, path)
	c.Assert(osenv.JujuHomeDir(), gc.Equals, path)
}

func (s *varsSuite) TestBlankJujuHomeEnvVar(c *gc.C) {
	s.PatchEnvironment(osenv.JujuHomeEnvKey, "")

	if runtime.GOOS == "windows" {
		s.PatchEnvironment("APPDATA", `P:\foobar`)
	} else {
		s.PatchEnvironment("HOME", "/foobar")
	}
	c.Assert(osenv.JujuHomeDir(), gc.Not(gc.Equals), "")

	if runtime.GOOS == "windows" {
		c.Assert(osenv.JujuHomeDir(), gc.Equals, osenv.JujuHomeWin())
	} else {
		c.Assert(osenv.JujuHomeDir(), gc.Equals, osenv.JujuHomeLinux())
	}
}

func (s *varsSuite) TestMergeEnvironment(c *gc.C) {
	c.Check(osenv.MergeEnvironment(nil, nil), gc.HasLen, 0)
	newValues := map[string]string{"a": "baz", "c": "omg"}
	created := osenv.MergeEnvironment(nil, newValues)
	expected := map[string]string{"a": "baz", "c": "omg"}
	c.Check(created, jc.DeepEquals, expected)
	// Show that the map returned isn't the one passed in.
	newValues["d"] = "another"
	c.Check(created, jc.DeepEquals, expected)
}

func (s *varsSuite) TestMergeEnvWin(c *gc.C) {
	initial := map[string]string{"a": "foo", "b": "bar", "foo": "val"}
	newValues := map[string]string{"a": "baz", "c": "omg", "FOO": "val2", "d": "another"}

	created := osenv.MergeEnvWin(initial, newValues)
	expected := map[string]string{"a": "baz", "b": "bar", "c": "omg", "foo": "val2", "d": "another"}
	// The returned value is the inital map.
	c.Check(created, jc.DeepEquals, expected)
	c.Check(initial, jc.DeepEquals, expected)
}

func (s *varsSuite) TestMergeEnvUnix(c *gc.C) {
	initial := map[string]string{"a": "foo", "b": "bar"}
	newValues := map[string]string{"a": "baz", "c": "omg", "d": "another"}

	created := osenv.MergeEnvUnix(initial, newValues)
	expected := map[string]string{"a": "baz", "b": "bar", "c": "omg", "d": "another"}
	// The returned value is the inital map.
	c.Check(created, jc.DeepEquals, expected)
	c.Check(initial, jc.DeepEquals, expected)
}
