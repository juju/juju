package osenv_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing/testbase"
)

func (*importSuite) TestHomeLinux(c *gc.C) {
	h := "/home/foo/bar"
	testbase.PatchEnvironment("HOME", h)
	c.Check(osenv.Home(), gc.Equals, h)
}
