// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"os"
	"reflect"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type CmdSuite struct {
	testing.JujuConnSuite
	home *coretesting.FakeHome
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
	s.home = coretesting.MakeFakeHome(c, envConfig, "peckham", "walthamstow", "brokenenv")
}

func (s *CmdSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
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

// assertConnName asserts that the Command is using
// the given environment name.
// Since every command has a different type,
// we use reflection to look at the value of the
// Conn field in the value.
func assertConnName(c *gc.C, com cmd.Command, name string) {
	v := reflect.ValueOf(com).Elem().FieldByName("EnvName")
	c.Assert(v, jc.Satisfies, reflect.Value.IsValid)
	c.Assert(v.Interface(), gc.Equals, name)
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
func (*CmdSuite) TestEnvironmentInit(c *gc.C) {
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
		if _, ok := com.(*StatusCommand); !ok {
			testInit(c, com, append(args, "hotdog"), "unrecognized args.*")
		}
	}
}

func runCommand(ctx *cmd.Context, com cmd.Command, args ...string) (opc chan dummy.Operation, errc chan error) {
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

		if ctx == nil {
			ctx = cmd.DefaultContext()
		}
		err = com.Run(ctx)
		errc <- err
	}()
	return
}

func (*CmdSuite) TestDestroyEnvironmentCommand(c *gc.C) {
	// Prepare the environment so we can destroy it.
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	_, err = environs.PrepareFromName("", store)
	c.Assert(err, gc.IsNil)

	// Verify that the environment information exists.
	_, err = store.ReadInfo("peckham")
	c.Assert(err, gc.IsNil)

	// normal destroy
	opc, errc := runCommand(nil, new(DestroyEnvironmentCommand), "--yes")
	c.Check(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "peckham")

	// Verify that the environment information has been removed.
	_, err = store.ReadInfo("peckham")
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	_, err = environs.PrepareFromName("brokenenv", store)
	c.Assert(err, gc.IsNil)

	// destroy with broken environment
	opc, errc = runCommand(nil, new(DestroyEnvironmentCommand), "--yes", "-e", "brokenenv")
	c.Check(<-opc, gc.IsNil)
	c.Check(<-errc, gc.ErrorMatches, "dummy.Destroy is broken")
	c.Check(<-opc, gc.IsNil)
}

func (*CmdSuite) TestDestroyEnvironmentCommandConfirmationFlag(c *gc.C) {
	com := new(DestroyEnvironmentCommand)
	c.Check(coretesting.InitCommand(com, nil), gc.IsNil)
	c.Check(com.assumeYes, gc.Equals, false)

	com = new(DestroyEnvironmentCommand)
	c.Check(coretesting.InitCommand(com, []string{"-y"}), gc.IsNil)
	c.Check(com.assumeYes, gc.Equals, true)

	com = new(DestroyEnvironmentCommand)
	c.Check(coretesting.InitCommand(com, []string{"--yes"}), gc.IsNil)
	c.Check(com.assumeYes, gc.Equals, true)
}

func (*CmdSuite) TestDestroyEnvironmentCommandConfirmation(c *gc.C) {
	var stdin, stdout bytes.Buffer
	ctx := cmd.DefaultContext()
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	// Prepare the environment so we can destroy it.
	env, err := environs.PrepareFromName("", store)
	c.Assert(err, gc.IsNil)

	assertEnvironNotDestroyed(c, env, store)

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString("n")
	opc, errc := runCommand(ctx, new(DestroyEnvironmentCommand))
	c.Check(<-errc, gc.ErrorMatches, "Environment destruction aborted")
	c.Check(<-opc, gc.IsNil)
	c.Check(stdout.String(), gc.Matches, "WARNING:.*peckham.*\\(type: dummy\\)(.|\n)*")
	assertEnvironNotDestroyed(c, env, store)

	// EOF on stdin: equivalent to answering no.
	stdin.Reset()
	stdout.Reset()
	opc, errc = runCommand(ctx, new(DestroyEnvironmentCommand))
	c.Check(<-opc, gc.IsNil)
	c.Check(<-errc, gc.ErrorMatches, "Environment destruction aborted")
	assertEnvironNotDestroyed(c, env, store)

	// "--yes" passed: no confirmation request.
	stdin.Reset()
	stdout.Reset()
	opc, errc = runCommand(ctx, new(DestroyEnvironmentCommand), "--yes")
	c.Check(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "peckham")
	c.Check(stdout.String(), gc.Equals, "")
	assertEnvironDestroyed(c, env, store)

	// Any of casing of "y" and "yes" will confirm.
	for _, answer := range []string{"y", "Y", "yes", "YES"} {
		// Prepare the environment so we can destroy it.
		_, err := environs.PrepareFromName("", store)
		c.Assert(err, gc.IsNil)

		stdin.Reset()
		stdout.Reset()
		stdin.WriteString(answer)
		opc, errc = runCommand(ctx, new(DestroyEnvironmentCommand))
		c.Check(<-errc, gc.IsNil)
		c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "peckham")
		c.Check(stdout.String(), gc.Matches, "WARNING:.*peckham.*\\(type: dummy\\)(.|\n)*")
		assertEnvironDestroyed(c, env, store)
	}
}

func assertEnvironDestroyed(c *gc.C, env environs.Environ, store configstore.Storage) {
	_, err := store.ReadInfo(env.Name())
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	c.Assert(func() { env.Instances([]instance.Id{"invalid"}) }, gc.PanicMatches, `environment.*is not prepared`)
}

func assertEnvironNotDestroyed(c *gc.C, env environs.Environ, store configstore.Storage) {
	info, err := store.ReadInfo(env.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(info.Initialized(), jc.IsTrue)

	_, err = environs.NewFromName(env.Name())
	c.Assert(err, gc.IsNil)
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

func (*CmdSuite) TestDeployCommandInit(c *gc.C) {
	defer os.Setenv(osenv.JujuRepository, os.Getenv(osenv.JujuRepository))
	os.Setenv(osenv.JujuRepository, "/path/to/repo")

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
	c.Assert(err, gc.ErrorMatches, "no service name specified")
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

func initDestroyUnitCommand(args ...string) (*DestroyUnitCommand, error) {
	com := &DestroyUnitCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestDestroyUnitCommandInit(c *gc.C) {
	// missing args
	_, err := initDestroyUnitCommand()
	c.Assert(err, gc.ErrorMatches, "no units specified")
	// not a unit
	_, err = initDestroyUnitCommand("seven/nine")
	c.Assert(err, gc.ErrorMatches, `invalid unit name "seven/nine"`)
}
