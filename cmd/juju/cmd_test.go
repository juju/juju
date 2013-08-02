// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"
	"reflect"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
)

type CmdSuite struct {
	testing.JujuConnSuite
	home *coretesting.FakeHome
}

var _ = Suite(&CmdSuite{})

const envConfig = `
default:
    peckham
environments:
    peckham:
        type: dummy
        state-server: false
        admin-secret: arble
        authorized-keys: i-am-a-key
        default-series: defaultseries
    walthamstow:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
    brokenenv:
        type: dummy
        broken: Bootstrap Destroy
        state-server: false
        authorized-keys: i-am-a-key
`

func (s *CmdSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.home = coretesting.MakeFakeHome(c, envConfig, "peckham", "walthamstow", "brokenenv")
}

func (s *CmdSuite) TearDownTest(c *C) {
	s.home.Restore()
	s.JujuConnSuite.TearDownTest(c)
}

// testInit checks that a command initialises correctly
// with the given set of arguments.
func testInit(c *C, com cmd.Command, args []string, errPat string) {
	err := coretesting.InitCommand(com, args)
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
	c.Assert(v, checkers.Satisfies, reflect.Value.IsValid)
	c.Assert(v.Interface(), Equals, name)
}

// All members of EnvironmentInitTests are tested for the -environment and -e
// flags, and that extra arguments will cause parsing to fail.
var EnvironmentInitTests = []func() (cmd.Command, []string){
	func() (cmd.Command, []string) { return new(BootstrapCommand), nil },
	func() (cmd.Command, []string) { return new(DestroyEnvironmentCommand), nil },
	func() (cmd.Command, []string) {
		return new(DeployCommand), []string{"charm-name", "service-name"}
	},
	func() (cmd.Command, []string) { return new(StatusCommand), nil },
}

// TestEnvironmentInit tests that all commands which accept
// the --environment variable initialise their
// environment name correctly.
func (*CmdSuite) TestEnvironmentInit(c *C) {
	for i, cmdFunc := range EnvironmentInitTests {
		c.Logf("test %d", i)
		com, args := cmdFunc()
		testInit(c, com, args, "")
		assertConnName(c, com, "")

		com, args = cmdFunc()
		testInit(c, com, append(args, "-e", "walthamstow"), "")
		assertConnName(c, com, "walthamstow")

		com, args = cmdFunc()
		testInit(c, com, append(args, "--environment", "walthamstow"), "")
		assertConnName(c, com, "walthamstow")

		// JUJU_ENV is the final place the environment can be overriden
		com, args = cmdFunc()
		oldenv := os.Getenv(osenv.JujuEnv)
		os.Setenv(osenv.JujuEnv, "walthamstow")
		testInit(c, com, args, "")
		os.Setenv(osenv.JujuEnv, oldenv)
		assertConnName(c, com, "walthamstow")

		com, args = cmdFunc()
		testInit(c, com, append(args, "hotdog"), "unrecognized args.*")
	}
}

func runCommand(com cmd.Command, args ...string) (opc chan dummy.Operation, errc chan error) {
	errc = make(chan error, 1)
	opc = make(chan dummy.Operation, 200)
	dummy.Listen(opc)
	go func() {
		// signal that we're done with this ops channel.
		defer dummy.Listen(nil)

		err := coretesting.InitCommand(com, args)
		if err != nil {
			errc <- err
			return
		}

		err = com.Run(cmd.DefaultContext())
		errc <- err
	}()
	return
}

func (*CmdSuite) TestDestroyEnvironmentCommand(c *C) {
	// normal destroy
	opc, errc := runCommand(new(DestroyEnvironmentCommand))
	c.Check(<-errc, IsNil)
	c.Check((<-opc).(dummy.OpDestroy).Env, Equals, "peckham")

	// destroy with broken environment
	opc, errc = runCommand(new(DestroyEnvironmentCommand), "-e", "brokenenv")
	c.Check(<-opc, IsNil)
	c.Check(<-errc, ErrorMatches, "dummy.Destroy is broken")
	c.Check(<-opc, IsNil)
}

var deployTests = []struct {
	args []string
	com  *DeployCommand
}{
	{
		[]string{"charm-name"},
		&DeployCommand{},
	}, {
		[]string{"charm-name", "service-name"},
		&DeployCommand{ServiceName: "service-name"},
	}, {
		[]string{"--repository", "/path/to/another-repo", "charm-name"},
		&DeployCommand{RepoPath: "/path/to/another-repo"},
	}, {
		[]string{"--upgrade", "charm-name"},
		&DeployCommand{BumpRevision: true},
	}, {
		[]string{"-u", "charm-name"},
		&DeployCommand{BumpRevision: true},
	}, {
		[]string{"--num-units", "33", "charm-name"},
		&DeployCommand{UnitCommandBase: UnitCommandBase{NumUnits: 33}},
	}, {
		[]string{"-n", "104", "charm-name"},
		&DeployCommand{UnitCommandBase: UnitCommandBase{NumUnits: 104}},
	},
}

func initExpectations(com *DeployCommand) {
	if com.CharmName == "" {
		com.CharmName = "charm-name"
	}
	if com.NumUnits == 0 {
		com.NumUnits = 1
	}
	if com.RepoPath == "" {
		com.RepoPath = "/path/to/repo"
	}
}

func initDeployCommand(args ...string) (*DeployCommand, error) {
	com := &DeployCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestDeployCommandInit(c *C) {
	defer os.Setenv(osenv.JujuRepository, os.Getenv(osenv.JujuRepository))
	os.Setenv(osenv.JujuRepository, "/path/to/repo")

	for _, t := range deployTests {
		initExpectations(t.com)
		com, err := initDeployCommand(t.args...)
		c.Assert(err, IsNil)
		c.Assert(com, DeepEquals, t.com)
	}

	// test relative --config path
	ctx := coretesting.Context(c)
	expected := []byte("test: data")
	path := ctx.AbsPath("testconfig.yaml")
	file, err := os.Create(path)
	c.Assert(err, IsNil)
	_, err = file.Write(expected)
	c.Assert(err, IsNil)
	file.Close()

	com, err := initDeployCommand("--config", "testconfig.yaml", "charm-name")
	c.Assert(err, IsNil)
	actual, err := com.Config.Read(ctx)
	c.Assert(err, IsNil)
	c.Assert(expected, DeepEquals, actual)

	// missing args
	_, err = initDeployCommand()
	c.Assert(err, ErrorMatches, "no charm specified")

	// environment tested elsewhere
}

func initAddUnitCommand(args ...string) (*AddUnitCommand, error) {
	com := &AddUnitCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestAddUnitCommandInit(c *C) {
	// missing args
	_, err := initAddUnitCommand()
	c.Assert(err, ErrorMatches, "no service specified")

	// bad unit count
	_, err = initDeployCommand("charm-name", "--num-units", "0")
	c.Assert(err, ErrorMatches, "--num-units must be a positive integer")
	_, err = initDeployCommand("charm-name", "-n", "0")
	c.Assert(err, ErrorMatches, "--num-units must be a positive integer")

	// environment tested elsewhere
}

func initExposeCommand(args ...string) (*ExposeCommand, error) {
	com := &ExposeCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestExposeCommandInit(c *C) {
	// missing args
	_, err := initExposeCommand()
	c.Assert(err, ErrorMatches, "no service name specified")

	// environment tested elsewhere
}

func initUnexposeCommand(args ...string) (*UnexposeCommand, error) {
	com := &UnexposeCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestUnexposeCommandInit(c *C) {
	// missing args
	_, err := initUnexposeCommand()
	c.Assert(err, ErrorMatches, "no service name specified")

	// environment tested elsewhere
}

func initSSHCommand(args ...string) (*SSHCommand, error) {
	com := &SSHCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestSSHCommandInit(c *C) {
	// missing args
	_, err := initSSHCommand()
	c.Assert(err, ErrorMatches, "no service name specified")
}

func initSCPCommand(args ...string) (*SCPCommand, error) {
	com := &SCPCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestSCPCommandInit(c *C) {
	// missing args
	_, err := initSCPCommand()
	c.Assert(err, ErrorMatches, "at least two arguments required")

	// not enough args
	_, err = initSCPCommand("mysql/0:foo")
	c.Assert(err, ErrorMatches, "at least two arguments required")
}

func initGetCommand(args ...string) (*GetCommand, error) {
	com := &GetCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestGetCommandInit(c *C) {
	// missing args
	_, err := initGetCommand()
	c.Assert(err, ErrorMatches, "no service name specified")
}

func initSetCommand(args ...string) (*SetCommand, error) {
	com := &SetCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestSetCommandInit(c *C) {
	// missing args
	_, err := initSetCommand()
	c.Assert(err, ErrorMatches, "no service name specified")
	// missing service name
	_, err = initSetCommand("name=cow")
	c.Assert(err, ErrorMatches, "no service name specified")

	// test --config path
	expected := []byte("this: is some test data")
	ctx := coretesting.Context(c)
	path := ctx.AbsPath("testconfig.yaml")
	file, err := os.Create(path)
	c.Assert(err, IsNil)
	_, err = file.Write(expected)
	c.Assert(err, IsNil)
	file.Close()
	com, err := initSetCommand("--config", "testconfig.yaml", "service")
	c.Assert(err, IsNil)
	c.Assert(com.SettingsYAML.Path, Equals, "testconfig.yaml")
	actual, err := com.SettingsYAML.Read(ctx)
	c.Assert(err, IsNil)
	c.Assert(actual, DeepEquals, expected)

	// --config path, but no service
	com, err = initSetCommand("--config", "testconfig")
	c.Assert(err, ErrorMatches, "no service name specified")

	// --config and options specified
	com, err = initSetCommand("service", "--config", "testconfig", "bees=")
	c.Assert(err, ErrorMatches, "cannot specify --config when using key=value arguments")
}

func initDestroyUnitCommand(args ...string) (*DestroyUnitCommand, error) {
	com := &DestroyUnitCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestDestroyUnitCommandInit(c *C) {
	// missing args
	_, err := initDestroyUnitCommand()
	c.Assert(err, ErrorMatches, "no units specified")
	// not a unit
	_, err = initDestroyUnitCommand("seven/nine")
	c.Assert(err, ErrorMatches, `invalid unit name "seven/nine"`)
}
