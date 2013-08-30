package osenv_test

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
)

func (*importSuite) TestHome(c *gc.C) {
	oldhome := os.Getenv("HOMEPATH")
	olddrive := os.Getenv("HOMEDRIVE")
	defer func() {
		os.Setenv("HOMEPATH", oldhome)
		os.Setenv("HOMEDRIVE", olddrive)
	}()
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
