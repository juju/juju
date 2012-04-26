package main_test

import (
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/juju"
	"launchpad.net/juju/go/environs/dummy"
	"os"
	"path/filepath"
	"reflect"
)

type cmdSuite struct {
	home string
}

var _ = Suite(&cmdSuite{})

// N.B. Barking is broken.
var config = `
default:
    peckham
environments:
    peckham:
        type: dummy
        zookeeper: false
    walthamstow:
        type: dummy
        zookeeper: false
    barking:
        type: dummy
        broken: true
        zookeeper: false
`

func (s *cmdSuite) SetUpTest(c *C) {
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

func (s *cmdSuite) TearDownTest(c *C) {
	os.Setenv("HOME", s.home)

	dummy.Reset(nil)
}

func newFlagSet() *gnuflag.FlagSet {
	return gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
}

// testInit checks that a command initialises correctly
// with the given set of arguments.
func testInit(c *C, com cmd.Command, args []string, errPat string) {
	err := com.Init(newFlagSet(), args)
	if errPat != "" {
		c.Assert(err, ErrorMatches, errPat)
	} else {
		c.Assert(err, IsNil)
	}
}

// assertConnName asserts that the Command is using
// the given environment name.
// Since every command has a different type,
// we use reflection to look at the value of the
// Conn field in the value.
func assertConnName(c *C, com cmd.Command, name string) {
	v := reflect.ValueOf(com).Elem().FieldByName("EnvName")
	c.Assert(v.IsValid(), Equals, true)
	c.Assert(v.Interface(), Equals, name)
}

// All members of EnvironmentInitTests are tested for the -environment and -e
// flags, and that extra arguments will cause parsing to fail.
var EnvironmentInitTests = []func() cmd.Command{
	func() cmd.Command { return &main.BootstrapCommand{} },
	func() cmd.Command { return &main.DestroyCommand{} },
}

// TestEnvironmentInit tests that all commands which accept
// the --environment variable initialise their
// environment name correctly.
func (*cmdSuite) TestEnvironmentInit(c *C) {
	for i, cmdFunc := range EnvironmentInitTests {
		c.Logf("test %d", i)
		com := cmdFunc()
		testInit(c, com, nil, "")
		assertConnName(c, com, "")

		com = cmdFunc()
		testInit(c, com, []string{"-e", "walthamstow"}, "")
		assertConnName(c, com, "walthamstow")

		com = cmdFunc()
		testInit(c, com, []string{"--environment", "walthamstow"}, "")
		assertConnName(c, com, "walthamstow")

		com = cmdFunc()
		testInit(c, com, []string{"hotdog"}, "unrecognised args.*")
	}
}

var CommandsTests = []struct {
	cmd     cmd.Command
	args    []string          // Arguments to give to command.
	ops     []dummy.Operation // Expected operations performed by command.
	initErr string            // Expected error from Init.
	runErr  string            // Expected error from Run.
}{
	{
		cmd:     &main.BootstrapCommand{},
		args:    []string{"hotdog"},
		initErr: `unrecognised args: \[hotdog\]`,
	}, {
		cmd: &main.BootstrapCommand{},
		ops: envOps("peckham", dummy.OpBootstrap),
	}, {
		cmd:    &main.BootstrapCommand{},
		args:   []string{"-e", "barking"},
		ops:    envOps("barking", dummy.OpBootstrap),
		runErr: "broken environment",
	}, {
		cmd: &main.DestroyCommand{},
		ops: envOps("peckham", dummy.OpDestroy),
	}, {
		cmd:    &main.DestroyCommand{},
		args:   []string{"-e", "barking"},
		ops:    envOps("barking", dummy.OpDestroy),
		runErr: "broken environment",
	},
}

func (*cmdSuite) TestCommands(c *C) {
	for i, t := range CommandsTests {
		c.Logf("test %d: %T", i, t.cmd)

		// Gather operations as they happen.
		opc := make(chan dummy.Operation)
		done := make(chan bool)
		var ops []dummy.Operation
		go func() {
			for op := range opc {
				ops = append(ops, op)
			}
			done <- true
		}()
		dummy.Reset(opc)

		testInit(c, t.cmd, t.args, t.initErr)
		if t.initErr != "" {
			// It's already passed the test in testParse.
			continue
		}

		err := t.cmd.Run(cmd.DefaultContext())
		// signal that we're done with this listener channel.
		dummy.Reset(nil)
		<-done
		if t.runErr != "" {
			c.Assert(err, ErrorMatches, t.runErr)
			continue
		}
		c.Assert(err, IsNil)
		c.Check(ops, DeepEquals, t.ops)
	}
}

// envOps returns a slice of expected operations on a given
// environment name.
func envOps(name string, events ...dummy.OperationKind) []dummy.Operation {
	ops := make([]dummy.Operation, len(events))
	for i, e := range events {
		ops[i] = dummy.Operation{
			EnvironName: name,
			Kind:        e,
		}
	}
	return ops
}
