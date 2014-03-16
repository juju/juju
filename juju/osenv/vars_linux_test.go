package osenv_test

import (
	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
)

func (*importSuite) TestHomeLinux(c *gc.C) {
	h := "/home/foo/bar"
	testing.PatchEnvironment("HOME", h)
	c.Check(osenv.Home(), gc.Equals, h)
}
