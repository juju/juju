package main_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/juju"
	"launchpad.net/juju/go/environs/dummy"
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
        type: dummy
        zookeeper: false
    walthamstow:
        type: dummy
        zookeeper: false
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

	dummy.Reset(nil)
}

type cmdTest struct {
	description string
	cmd      cmd.Command
	args     []string
	ops      []dummy.Operation
	parseErr string
	runErr   string
}

var cmdTests = []cmdTest {
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
		envOps("peckham", dummy.OpBootstrap),
		"",
		"",
	}, {
		&main.BootstrapCommand{},
		[]string{"-e", "walthamstow"},
		envOps("walthamstow", dummy.OpBootstrap),
		"",
		"",
	}, {
		&main.BootstrapCommand{},
		[]string{"-e", "walthamstow"},
		envOps("walthamstow", dummy.OpBootstrap),
		"",
		"",
	}, {
		&main.DestroyCommand{},
		[]string{},
		envOps("peckham", dummy.OpDestroy),
		"",
		"",
	}, {
		&main.DestroyCommand{},
		[]string{"-e", "walthamstow"},
		envOps("walthamstow", dummy.OpDestroy),
		"",
		"",
	},
}

// All members of genericTests are tested for the -environment and -e
// flags, and that extra arguments will cause parsing to fail.
var genericParseTests = []struct {
	cmd cmd.Command
	args []string
	allowsExtraArgs
} {{
		cmd: &main.Bootstrap.Command{},
	}, {
		cmd: &main.DestroyCommand{},
	},
}

func newCommand(old cmd.Command) cmd.Command {
	v := reflect.New(reflect.TypeOf(old).Elem())
	return v.Interface().(cmd.Command)
}

	args     []string
	ops      []dummy.Operation
	parseErr string
	runErr   string

func testParse(c *C, com cmd.Command, args []string, errPat string) cmd.Command {
	com = newCommand(com)
	err := cmd.Parse(com, args)
	if err != nil {
		c.Assert(err, ErrorMatches, errPat)
	} else {
		c.Assert(err, IsNil)
	}
	return com
}

func testConn(c *C, com cmd.Command, name string) {
	v := reflect.NewValue(com).Elem().FieldByName("Conn")
	c.Assert(v.IsValid(), Equals, true)
	conn := v.Interface().(*juju.Conn)
	c.Assert(dummy.EnvironName(conn.Environ), Equals, name)
}

func (*suite) TestGeneric() {
	for _, t := range genericTests {
		com := testParse(c, t.cmd, t.args, "")
		testConn(c, com, "peckham")

		com = testParse(c, t.cmd, append([]string{"-e", "walthamstow"}, args), "")
		testConn(c, com, "walthamstow")

		com = testParse(c, t.cmd, append([]string{"-environment", "walthamstow"}, args), "")
		testConn(c, com, "walthamstow")

		testParse(c, t.cmd, append([]string{"-e", "unknown"}, args), "some error")
	}
}

func (*suite) TestCommands(c *C) {
	for i, t := range cmdTests {
		c.Log("test %d", i)
		err := cmd.Parse(t.cmd, t.args)
		checkError(c, "parse", err, t.parseErr)
	
		// gather operations as they happen
		opc := make(chan dummy.Operation)
		dummy.Reset(opc)
		done := make(chan bool)
		var ops []dummy.Operation
		go func() {
			for op := range opc {
				ops = append(ops, op)
			}
			done <- true
		}()
	
		err = t.cmd.Run()
		checkError(c, "run", err, t.runErr)
	
		// signal that we're done with this listener channel.
		dummy.Reset(nil)
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

// envOps returns a slice of expected operations on a given
// environment name. The returned slice always starts
// with dummy.OpOpen.
func envOps(name string, events ...dummy.OperationKind) []dummy.Operation {
	ops := make([]dummy.Operation, len(events)+1)
	ops[0] = dummy.Operation{
		EnvironName: name,
		Kind: dummy.OpOpen,
	}
	for i, e := range events {
		ops[i+1] = dummy.Operation{
			EnvironName: name,
			Kind: e,
		}
	}
	return ops
}
