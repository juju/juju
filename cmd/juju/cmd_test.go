package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/juju"
	"launchpad.net/juju/go/testing"
	"io/ioutil"
	"os"
	"path/filepath"
)

type suite struct {
	home string
}

var _ = Suite(suite{})

var config = `
default:
    peckham
environments:
    peckham:
        type: testing
        name: peckham
    walthamstow:
        type: testing
        name: walthamstow
`

func (s *suite) SetUpTest(c *C) {
	// Arrange so that the "home" directory points
	// to a temporary directory containing the config file.
	s.home = os.Getenv("HOME")
	dir := c.MkDir()
	os.Setenv("HOME", dir)
	err := os.Mkdir(filepath.Join(dir, ".juju"), 0777)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(dir, ".juju", "environments.yaml"), []byte(config), 0666)
	c.Assert(err, IsNil)
}

func (s *suite) TearDownTest(c *C) {
	os.Setenv("HOME", s.home)

	// enable other tests to use the testing environment with impunity.
	testing.ListenEnvirons(testing.DiscardEnvOps)
}

var cmdTests = []struct {
	cmd cmd.Command
	args []string
	ops []testing.EnvOp
	parseErr string
	runErr string
} {
// In the first few tests, we fully test the --environment
// flag. In the others we do only a rudimentary test,
// because we know that it's always implemented by
// embedding a conn type, so checking that -e
// works correctly is sufficient to infer that the other
// behaviours work too.
{
	&main.BootstrapCommand{},
	[]string{"hotdog"},
	nil,
	`unrecognised args: \[hotdog\]`,
	"",
}, {
	&main.BootstrapCommand{},
	[]string{},
	envOps("peckham", testing.EnvBootstrap),
	"",
	"",
}, {
	&main.BootstrapCommand{},
	[]string{"-e", "walthamstow"},
	envOps("walthamstow", testing.EnvBootstrap),
	"",
	"",
}, {
	&main.BootstrapCommand{},
	[]string{"-e", "walthamstow"},
	envOps("walthamstow", testing.EnvBootstrap),
	"",
	"",
},
}

func (*suite) TestCommands(c *C) {
	for i, t := range cmdTests {
		c.Log("test %d", i)
		err := cmd.Parse(t.cmd, t.args)
		checkError(c, "parse", err, t.parseErr)

		// gather operations as they happen
		opc := make(chan testing.EnvOp)
		testing.ListenEnvirons(opc)
		done := make(chan bool)
		var ops []testing.EnvOp
		go func() {
			for op := range opc {
				ops = append(ops, op)
			}
			done <- true
		}()

		err = t.cmd.Run()
		checkError(c, "run", err, t.runErr)

		// signal that we're done with this listener channel.
		testing.ListenEnvirons(nil)
		<-done

		c.Check(ops, DeepEquals, t.ops)
	}
}

func checkError(c *C, kind string, err error, expect string) {
	switch {
	case err != nil && expect == "":
		c.Fatalf("unexpected %s error: %v", kind, err)
	case err != nil && expect != "":
		c.Assert(err, ErrorMatches, expect)
	case err == nil && expect != "":
		c.Fatalf("unexpected %s success: expected %q", kind, expect)
	}
}

func envOps(name string, events ...testing.EnvOpKind) []testing.EnvOp {
	ops := make([]testing.EnvOp, len(events))
	for i, e := range events {
		ops[i] = testing.EnvOp{
			Name: name,
			Kind: e,
		}
	}
	return ops
}
