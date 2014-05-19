// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io"
	"io/ioutil"
	"os"
	"reflect"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
)

type CmdSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&CmdSuite{})

const envConfig = `
default:
    peckham
environments:
    peckham:
        type: dummy
        state-server: false
        admin-secret: arble
        authorized-keys: i-am-a-key
        default-series: raring
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

func (s *CmdSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	coretesting.WriteEnvironments(c, envConfig, "peckham", "walthamstow", "brokenenv")
}

func (s *CmdSuite) TearDownTest(c *gc.C) {
	s.JujuConnSuite.TearDownTest(c)
}

// testInit checks that a command initialises correctly
// with the given set of arguments.
func testInit(c *gc.C, com cmd.Command, args []string, errPat string) {
	err := coretesting.InitCommand(com, args)
	if errPat != "" {
		c.Assert(err, gc.ErrorMatches, errPat)
	} else {
		c.Assert(err, gc.IsNil)
	}
}

// assertEnvName asserts that the Command is using
// the given environment name.
// Since every command has a different type,
// we use reflection to look at the value of the
// Conn field in the value.
func assertEnvName(c *gc.C, com cmd.Command, name string) {
	v := reflect.ValueOf(com).Elem().FieldByName("EnvName")
	c.Assert(v, jc.Satisfies, reflect.Value.IsValid)
	c.Assert(v.Interface(), gc.Equals, name)
}

// All members of EnvironmentInitTests are tested for the -environment and -e
// flags, and that extra arguments will cause parsing to fail.
var EnvironmentInitTests = []func() (envcmd.EnvironCommand, []string){
	func() (envcmd.EnvironCommand, []string) { return new(BootstrapCommand), nil },
	func() (envcmd.EnvironCommand, []string) {
		return new(DeployCommand), []string{"charm-name", "service-name"}
	},
	func() (envcmd.EnvironCommand, []string) { return new(StatusCommand), nil },
}

// TestEnvironmentInit tests that all commands which accept
// the --environment variable initialise their
// environment name correctly.
func (*CmdSuite) TestEnvironmentInit(c *gc.C) {
	for i, cmdFunc := range EnvironmentInitTests {
		c.Logf("test %d", i)
		com, args := cmdFunc()
		testInit(c, envcmd.Wrap(com), args, "")
		assertEnvName(c, com, "peckham")

		com, args = cmdFunc()
		testInit(c, envcmd.Wrap(com), append(args, "-e", "walthamstow"), "")
		assertEnvName(c, com, "walthamstow")

		com, args = cmdFunc()
		testInit(c, envcmd.Wrap(com), append(args, "--environment", "walthamstow"), "")
		assertEnvName(c, com, "walthamstow")

		// JUJU_ENV is the final place the environment can be overriden
		com, args = cmdFunc()
		oldenv := os.Getenv(osenv.JujuEnvEnvKey)
		os.Setenv(osenv.JujuEnvEnvKey, "walthamstow")
		testInit(c, envcmd.Wrap(com), args, "")
		os.Setenv(osenv.JujuEnvEnvKey, oldenv)
		assertEnvName(c, com, "walthamstow")
	}
}

func nullContext(c *gc.C) *cmd.Context {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, gc.IsNil)
	ctx.Stdin = io.LimitReader(nil, 0)
	ctx.Stdout = ioutil.Discard
	ctx.Stderr = ioutil.Discard
	return ctx
}

func runCommand(ctx *cmd.Context, com cmd.Command, args ...string) (opc chan dummy.Operation, errc chan error) {
	if ctx == nil {
		panic("ctx == nil")
	}
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

		err = com.Run(ctx)
		errc <- err
	}()
	return
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
	com.EnvCommandBase.EnvName = "peckham"
}

func initDeployCommand(args ...string) (*DeployCommand, error) {
	com := &DeployCommand{}
	return com, coretesting.InitCommand(envcmd.Wrap(com), args)
}

func (*CmdSuite) TestDeployCommandInit(c *gc.C) {
	defer os.Setenv(osenv.JujuRepositoryEnvKey, os.Getenv(osenv.JujuRepositoryEnvKey))
	os.Setenv(osenv.JujuRepositoryEnvKey, "/path/to/repo")

	for _, t := range deployTests {
		initExpectations(t.com)
		com, err := initDeployCommand(t.args...)
		c.Assert(err, gc.IsNil)
		c.Assert(com, gc.DeepEquals, t.com)
	}

	// test relative --config path
	ctx := coretesting.Context(c)
	expected := []byte("test: data")
	path := ctx.AbsPath("testconfig.yaml")
	file, err := os.Create(path)
	c.Assert(err, gc.IsNil)
	_, err = file.Write(expected)
	c.Assert(err, gc.IsNil)
	file.Close()

	com, err := initDeployCommand("--config", "testconfig.yaml", "charm-name")
	c.Assert(err, gc.IsNil)
	actual, err := com.Config.Read(ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(expected, gc.DeepEquals, actual)

	// missing args
	_, err = initDeployCommand()
	c.Assert(err, gc.ErrorMatches, "no charm specified")

	// environment tested elsewhere
}

func initAddUnitCommand(args ...string) (*AddUnitCommand, error) {
	com := &AddUnitCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestAddUnitCommandInit(c *gc.C) {
	// missing args
	_, err := initAddUnitCommand()
	c.Assert(err, gc.ErrorMatches, "no service specified")

	// bad unit count
	_, err = initDeployCommand("charm-name", "--num-units", "0")
	c.Assert(err, gc.ErrorMatches, "--num-units must be a positive integer")
	_, err = initDeployCommand("charm-name", "-n", "0")
	c.Assert(err, gc.ErrorMatches, "--num-units must be a positive integer")

	// environment tested elsewhere
}

func initExposeCommand(args ...string) (*ExposeCommand, error) {
	com := &ExposeCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestExposeCommandInit(c *gc.C) {
	// missing args
	_, err := initExposeCommand()
	c.Assert(err, gc.ErrorMatches, "no service name specified")

	// environment tested elsewhere
}

func initUnexposeCommand(args ...string) (*UnexposeCommand, error) {
	com := &UnexposeCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestUnexposeCommandInit(c *gc.C) {
	// missing args
	_, err := initUnexposeCommand()
	c.Assert(err, gc.ErrorMatches, "no service name specified")

	// environment tested elsewhere
}

func initSSHCommand(args ...string) (*SSHCommand, error) {
	com := &SSHCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestSSHCommandInit(c *gc.C) {
	// missing args
	_, err := initSSHCommand()
	c.Assert(err, gc.ErrorMatches, "no target name specified")
}

func initSCPCommand(args ...string) (*SCPCommand, error) {
	com := &SCPCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestSCPCommandInit(c *gc.C) {
	// missing args
	_, err := initSCPCommand()
	c.Assert(err, gc.ErrorMatches, "at least two arguments required")

	// not enough args
	_, err = initSCPCommand("mysql/0:foo")
	c.Assert(err, gc.ErrorMatches, "at least two arguments required")
}

func initGetCommand(args ...string) (*GetCommand, error) {
	com := &GetCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestGetCommandInit(c *gc.C) {
	// missing args
	_, err := initGetCommand()
	c.Assert(err, gc.ErrorMatches, "no service name specified")
}

func initSetCommand(args ...string) (*SetCommand, error) {
	com := &SetCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestSetCommandInit(c *gc.C) {
	// missing args
	_, err := initSetCommand()
	c.Assert(err, gc.ErrorMatches, "no service name specified")
	// missing service name
	_, err = initSetCommand("name=cow")
	c.Assert(err, gc.ErrorMatches, "no service name specified")

	// test --config path
	expected := []byte("this: is some test data")
	ctx := coretesting.Context(c)
	path := ctx.AbsPath("testconfig.yaml")
	file, err := os.Create(path)
	c.Assert(err, gc.IsNil)
	_, err = file.Write(expected)
	c.Assert(err, gc.IsNil)
	file.Close()
	com, err := initSetCommand("--config", "testconfig.yaml", "service")
	c.Assert(err, gc.IsNil)
	c.Assert(com.SettingsYAML.Path, gc.Equals, "testconfig.yaml")
	actual, err := com.SettingsYAML.Read(ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, expected)

	// --config path, but no service
	com, err = initSetCommand("--config", "testconfig")
	c.Assert(err, gc.ErrorMatches, "no service name specified")

	// --config and options specified
	com, err = initSetCommand("service", "--config", "testconfig", "bees=")
	c.Assert(err, gc.ErrorMatches, "cannot specify --config when using key=value arguments")
}

func initUnsetCommand(args ...string) (*UnsetCommand, error) {
	com := &UnsetCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestUnsetCommandInit(c *gc.C) {
	// missing args
	_, err := initUnsetCommand()
	c.Assert(err, gc.ErrorMatches, "no service name specified")
}

func initRemoveUnitCommand(args ...string) (*RemoveUnitCommand, error) {
	com := &RemoveUnitCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestRemoveUnitCommandInit(c *gc.C) {
	// missing args
	_, err := initRemoveUnitCommand()
	c.Assert(err, gc.ErrorMatches, "no units specified")
	// not a unit
	_, err = initRemoveUnitCommand("seven/nine")
	c.Assert(err, gc.ErrorMatches, `invalid unit name "seven/nine"`)
}
