package osenv_test

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
)

func (*importSuite) TestHomeLinux(c *gc.C) {
	oldhome := os.Getenv("HOME")
	defer func() { os.Setenv("HOME", oldhome) }()

	h := "/home/foo/bar"
	osenv.SetHome(h)
	c.Check(os.Getenv("HOME"), gc.Equals, h)
	c.Check(osenv.Home(), gc.Equals, h)
}
