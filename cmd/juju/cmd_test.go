package main_test

import (
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/juju"
	"launchpad.net/juju/go/environs/dummy"
	"launchpad.net/juju/go/juju"
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

// newCommand makes a new Command of the same
// type as the old one.
func newCommand(old cmd.Command) cmd.Command {
	v := reflect.New(reflect.TypeOf(old).Elem())
	return v.Interface().(cmd.Command)
}

// testInit checks that a command initialises correctly
// with the given set of arguments. It copies the
// command so that we can run several tests on
// the same command type, and returns the newly
// copied and parsed command.
func testInit(c *C, com cmd.Command, args []string, errPat string) cmd.Command {
	com = newCommand(com)
	err := com.Init(newFlagSet(), args)
	if errPat != "" {
		c.Assert(err, ErrorMatches, errPat)
	} else {
		c.Assert(err, IsNil)
	}
	return com
}

// assertConnName asserts that the Command is using
// the given environment name.
// Since every command has a different type,
// we use reflection to look at the value of the
// Conn field in the value.
func assertConnName(c *C, com cmd.Command, name string) {
	v := reflect.ValueOf(com).Elem().FieldByName("Conn")
	c.Assert(v.IsValid(), Equals, true)
	conn := v.Interface().(*juju.Conn)
	c.Assert(dummy.EnvironName(conn.Environ), Equals, name)
}

// All members of genericTests are tested for the -environment and -e
// flags, and that extra arguments will cause parsing to fail.
var genericInitTests = []cmd.Command{
	&main.BootstrapCommand{},
	&main.DestroyCommand{},
}

// TestEnvironmentInit tests that all commands which accept
// the --environment variable initialise their
// environment connection correctly.
func (*cmdSuite) TestEnvironmentInit(c *C) {
	for i, command := range genericInitTests {
		c.Logf("test %d", i)
		com := testInit(c, command, nil, "")
		assertConnName(c, com, "peckham")

		com = testInit(c, command, []string{"-e", "walthamstow"}, "")
		assertConnName(c, com, "walthamstow")

		com = testInit(c, command, []string{"--environment", "walthamstow"}, "")
		assertConnName(c, com, "walthamstow")

		testInit(c, command, []string{"-e", "unknown"}, "unknown environment .*")

		testInit(c, command, []string{"hotdog"}, "unrecognised args.*")
	}
}

var cmdTests = []struct {
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
	for i, t := range cmdTests {
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

		com := testInit(c, t.cmd, t.args, t.initErr)
		if t.initErr != "" {
			// It's already passed the test in testParse.
			continue
		}

		err := com.Run(cmd.DefaultContext())
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
