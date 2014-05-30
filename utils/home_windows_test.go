package utils_test

import (
	"os"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type homeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&homeSuite{})

func (s *homeSuite) TestHome(c *gc.C) {
	s.PatchEnvironment("HOMEPATH", "")
	s.PatchEnvironment("HOMEDRIVE", "")

	drive := "P:"
	path := `\home\foo\bar`
	h := drive + path
	utils.SetHome(h)
	c.Check(os.Getenv("HOMEPATH"), gc.Equals, path)
	c.Check(os.Getenv("HOMEDRIVE"), gc.Equals, drive)
	c.Check(utils.Home(), gc.Equals, h)

	// now test that if we only set the path, we don't mess with the drive

	path2 := `\home\someotherfoo\bar`

	utils.SetHome(path2)

	c.Check(os.Getenv("HOMEPATH"), gc.Equals, path2)
	c.Check(os.Getenv("HOMEDRIVE"), gc.Equals, drive)
	c.Check(utils.Home(), gc.Equals, drive+path2)
}
