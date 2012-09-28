package uniter

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type FilterSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&FilterSuite{})

func (s *FilterSuite) TestFatal(c *C) {
	c.Fatalf("cjisljcesl")
}
