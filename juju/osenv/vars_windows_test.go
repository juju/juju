package osenv_test

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing"
)

func (*importSuite) TestHome(c *gc.C) {
	testbase.PatchEnvironment("HOMEPATH", "")
	testbase.PatchEnvironment("HOMEDRIVE", "")

	drive := "P:"
	path := `\home\foo\bar`
	h := drive + path
	osenv.SetHome(h)
	c.Check(os.Getenv("HOMEPATH"), gc.Equals, path)
	c.Check(os.Getenv("HOMEDRIVE"), gc.Equals, drive)
	c.Check(osenv.Home(), gc.Equals, h)

	// now test that if we only set the path, we don't mess with the drive

	path2 := `\home\someotherfoo\bar`

	osenv.SetHome(path2)

	c.Check(os.Getenv("HOMEPATH"), gc.Equals, path2)
	c.Check(os.Getenv("HOMEDRIVE"), gc.Equals, drive)
	c.Check(osenv.Home(), gc.Equals, drive+path2)
}
