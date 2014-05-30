package utils_test

import (
	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type homeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&homeSuite{})

func (s *homeSuite) TestHomeLinux(c *gc.C) {
	h := "/home/foo/bar"
	s.PatchEnvironment("HOME", h)
	c.Check(utils.Home(), gc.Equals, h)
}
