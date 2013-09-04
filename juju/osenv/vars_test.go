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
	path := `P:\FooBar\AppData`
	defer patchEnvironment("APPDATA", path)()
	c.Assert(jujuHomeWin(), gc.Equals, filepath.Join(path, "Juju"))
}

func (*importSuite) TestJujuHomeLinux(c *gc.C) {
	path := `/foo/bar/baz/`
	defer patchEnvironment("HOME", path)()
	c.Assert(jujuHomeLinux(), gc.Equals, filepath.Join(path, ".juju"))
}

func (*importSuite) TestJujuHomeEnvVar(c *gc.C) {
	path := "/foo/bar/baz"
	defer patchEnvironment(JujuHome, path)()
	c.Assert(JujuHomeDir(), gc.Equals, path)
}

func (*importSuite) TestBlankJujuHomeEnvVar(c *gc.C) {
	defer patchEnvironment(JujuHome, "")()

	if runtime.GOOS == "windows" {
		defer patchEnvironment("APPDATA", `P:\foobar`)()
	} else {
		defer patchEnvironment("HOME", "/foobar")()
	}
	c.Assert(JujuHomeDir(), gc.Not(gc.Equals), "")

	if runtime.GOOS == "windows" {
		c.Assert(JujuHomeDir(), gc.Equals, jujuHomeWin())
	} else {
		c.Assert(JujuHomeDir(), gc.Equals, jujuHomeLinux())
	}
}

// yes this is a copy of coretesting's PatchEnvironment
// but otherwise we get an import cycle
func patchEnvironment(name, value string) func() {
	oldValue := os.Getenv(name)
	os.Setenv(name, value)
	return func() { os.Setenv(name, oldValue) }
}
