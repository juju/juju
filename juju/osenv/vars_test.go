// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	gc "launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type importSuite struct{}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestJujuHomeWin(c *gc.C) {
	old := os.Getenv("APPDATA")
	defer func() {
		os.Setenv("APPDATA", old)
	}()
	path := `P:\FooBar\AppData`
	if err := os.Setenv("APPDATA", path); err != nil {
		c.Fatalf("Error setting APPDATA: %s", err)
	}
	c.Assert(jujuHomeWin(), gc.Equals, filepath.Join(path, "Juju"))
}

func (*importSuite) TestJujuHomeLinux(c *gc.C) {
	old := os.Getenv("HOME")
	defer func() {
		os.Setenv("HOME", old)
	}()
	path := `/foo/bar/baz`
	if err := os.Setenv("HOME", path); err != nil {
		c.Fatalf("Error setting HOME: %s", err)
	}
	c.Assert(jujuHomeLinux(), gc.Equals, filepath.Join(path, ".juju"))
}

func (*importSuite) TestJujuHomeEnvVar(c *gc.C) {
	old := os.Getenv(JujuHome)
	defer func() {
		os.Setenv(JujuHome, old)
	}()
	path := "/foo/bar/baz"
	if err := os.Setenv(JujuHome, path); err != nil {
		c.Fatalf("Error setting jujuhome: %s", err)
	}
	c.Assert(JujuHomeDir(), gc.Equals, path)
}

func (*importSuite) TestBlankJujuHomeEnvVar(c *gc.C) {
	old := os.Getenv(JujuHome)
	// set OS specific default juju home locations
	defer func() {
		os.Setenv(JujuHome, old)
	}()
	if err := os.Setenv(JujuHome, ""); err != nil {
		c.Fatalf("Error setting jujuhome: %s", err)
	}
	if runtime.GOOS == "windows" {
		c.Assert(JujuHomeDir(), gc.Equals, jujuHomeWin())
	} else {
		c.Assert(JujuHomeDir(), gc.Equals, jujuHomeLinux())
	}
}
