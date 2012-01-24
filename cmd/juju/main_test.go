package main_test

import (
	"flag"
	"fmt"
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/juju"
	"os"
	"os/exec"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type CommandSuite struct{}

var _ = Suite(&CommandSuite{})

// Note that we're using the plain "flag" package here, no need or use for gnuflag
var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func (s *CommandSuite) TestRunMain(c *C) {
	fmt.Println("HELLO", *flagRunMain)
	fmt.Println(os.Args)
	fmt.Println(flag.Args())
	if *flagRunMain {
		fmt.Println(os.Args)
		fmt.Println(flag.Args())
		main.Main(flag.Args())
	}
}

func command(cmd ...string) *exec.Cmd {
	args := append([]string{"-test.run", "TestRunMain", "-test.v", "-run-main", "--", "juju"}, cmd...)
	fmt.Println(args)
	return exec.Command(os.Args[0], args...)
}

func (s *CommandSuite) TestActuallyRun(c *C) {
	ps := command("bootstrap", "--cheese")
	output, _ := ps.CombinedOutput()
	c.Assert(string(output), Equals, "")

}
