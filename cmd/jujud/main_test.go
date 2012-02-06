package main_test

import (
	"flag"
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/jujud"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type MainSuite struct{}

var _ = Suite(&MainSuite{})

var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the jujud
// tool itself.
func TestRunMain(t *testing.T) {
	if *flagRunMain {
		main.Main(flag.Args())
	}
}

func (s *MainSuite) TestFails(c *C) {
	c.Fail()
}
