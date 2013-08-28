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
	drive := "C:"
	path := `\home\foo\bar`
	h := drive + path
	osenv.SetHome(h)
	c.Check(os.Getenv("HOMEPATH"), gc.Equals, path)
	c.Check(os.Getenv("HOMEDRIVE"), gc.Equals, drive)
	c.Check(osenv.Home(), gc.Equals, h)
}
