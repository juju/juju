// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"os"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/cmd/juju/status"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

func badrun(c *gc.C, exit int, args ...string) string {
	args = append([]string{"juju"}, args...)
	return cmdtesting.BadRun(c, exit, args...)
}

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
        agent-stream: proposed
    devenv:
        type: dummy
        state-server: false
        admin-secret: arble
        authorized-keys: i-am-a-key
        default-series: raring
        agent-stream: proposed
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
		c.Assert(err, jc.ErrorIsNil)
	}
}

type HasEnvironmentName interface {
	EnvName() string
}

// assertEnvName asserts that the Command is using
// the given environment name.
// Since every command has a different type,
// we use reflection to look at the value of the
// Conn field in the value.
func assertEnvName(c *gc.C, com cmd.Command, name string) {
	i, ok := com.(HasEnvironmentName)
	c.Assert(ok, jc.IsTrue)
	c.Assert(i.EnvName(), gc.Equals, name)
}

// All members of EnvironmentInitTests are tested for the -environment and -e
// flags, and that extra arguments will cause parsing to fail.
var EnvironmentInitTests = []func() (cmd.Command, []string){
	func() (cmd.Command, []string) {
		return newBootstrapCommand(), nil
	},
	func() (cmd.Command, []string) {
		return newDeployCommand(), []string{"charm-name", "service-name"}
	},
	func() (cmd.Command, []string) {
		return status.NewStatusCommand(), nil
	},
}

// TestEnvironmentInit tests that all commands which accept
// the --environment variable initialise their
// environment name correctly.
func (*CmdSuite) TestEnvironmentInit(c *gc.C) {
	for i, cmdFunc := range EnvironmentInitTests {
		c.Logf("test %d", i)
		com, args := cmdFunc()
		testInit(c, com, args, "")
		assertEnvName(c, com, "peckham")

		com, args = cmdFunc()
		testInit(c, com, append(args, "-e", "walthamstow"), "")
		assertEnvName(c, com, "walthamstow")

		com, args = cmdFunc()
		testInit(c, com, append(args, "--environment", "walthamstow"), "")
		assertEnvName(c, com, "walthamstow")

		// JUJU_ENV is the final place the environment can be overriden
		com, args = cmdFunc()
		oldenv := os.Getenv(osenv.JujuEnvEnvKey)
		os.Setenv(osenv.JujuEnvEnvKey, "walthamstow")
		testInit(c, com, args, "")
		os.Setenv(osenv.JujuEnvEnvKey, oldenv)
		assertEnvName(c, com, "walthamstow")
	}
}

var deployTests = []struct {
	args []string
	com  *deployCommand
}{
	{
		[]string{"charm-name"},
		&deployCommand{},
	}, {
		[]string{"charm-name", "service-name"},
		&deployCommand{ServiceName: "service-name"},
	}, {
		[]string{"--repository", "/path/to/another-repo", "charm-name"},
		&deployCommand{RepoPath: "/path/to/another-repo"},
	}, {
		[]string{"--upgrade", "charm-name"},
		&deployCommand{BumpRevision: true},
	}, {
		[]string{"-u", "charm-name"},
		&deployCommand{BumpRevision: true},
	}, {
		[]string{"--num-units", "33", "charm-name"},
		&deployCommand{UnitCommandBase: service.UnitCommandBase{NumUnits: 33}},
	}, {
		[]string{"-n", "104", "charm-name"},
		&deployCommand{UnitCommandBase: service.UnitCommandBase{NumUnits: 104}},
	},
}

func initExpectations(com *deployCommand) {
	if com.CharmOrBundle == "" {
		com.CharmOrBundle = "charm-name"
	}
	if com.NumUnits == 0 {
		com.NumUnits = 1
	}
	if com.RepoPath == "" {
		com.RepoPath = "/path/to/repo"
	}
	com.SetEnvName("peckham")
}

func initDeployCommand(args ...string) (*deployCommand, error) {
	com := &deployCommand{}
	return com, coretesting.InitCommand(envcmd.Wrap(com), args)
}

func (*CmdSuite) TestDeployCommandInit(c *gc.C) {
	defer os.Setenv(osenv.JujuRepositoryEnvKey, os.Getenv(osenv.JujuRepositoryEnvKey))
	os.Setenv(osenv.JujuRepositoryEnvKey, "/path/to/repo")

	for _, t := range deployTests {
		initExpectations(t.com)
		com, err := initDeployCommand(t.args...)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(com, gc.DeepEquals, t.com)
	}

	// test relative --config path
	ctx := coretesting.Context(c)
	expected := []byte("test: data")
	path := ctx.AbsPath("testconfig.yaml")
	file, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)
	_, err = file.Write(expected)
	c.Assert(err, jc.ErrorIsNil)
	file.Close()

	com, err := initDeployCommand("--config", "testconfig.yaml", "charm-name")
	c.Assert(err, jc.ErrorIsNil)
	actual, err := com.Config.Read(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(expected, gc.DeepEquals, actual)

	// missing args
	_, err = initDeployCommand()
	c.Assert(err, gc.ErrorMatches, "no charm or bundle specified")

	// bad unit count
	_, err = initDeployCommand("charm-name", "--num-units", "0")
	c.Assert(err, gc.ErrorMatches, "--num-units must be a positive integer")
	_, err = initDeployCommand("charm-name", "-n", "0")
	c.Assert(err, gc.ErrorMatches, "--num-units must be a positive integer")

	// environment tested elsewhere
}

func initExposeCommand(args ...string) (*exposeCommand, error) {
	com := &exposeCommand{}
	return com, coretesting.InitCommand(envcmd.Wrap(com), args)
}

func (*CmdSuite) TestExposeCommandInit(c *gc.C) {
	// missing args
	_, err := initExposeCommand()
	c.Assert(err, gc.ErrorMatches, "no service name specified")

	// environment tested elsewhere
}

func initUnexposeCommand(args ...string) (*unexposeCommand, error) {
	com := &unexposeCommand{}
	return com, coretesting.InitCommand(envcmd.Wrap(com), args)
}

func (*CmdSuite) TestUnexposeCommandInit(c *gc.C) {
	// missing args
	_, err := initUnexposeCommand()
	c.Assert(err, gc.ErrorMatches, "no service name specified")

	// environment tested elsewhere
}

func initSSHCommand(args ...string) (*sshCommand, error) {
	com := &sshCommand{}
	return com, coretesting.InitCommand(com, args)
}

func (*CmdSuite) TestSSHCommandInit(c *gc.C) {
	// missing args
	_, err := initSSHCommand()
	c.Assert(err, gc.ErrorMatches, "no target name specified")
}

func initSCPCommand(args ...string) (*scpCommand, error) {
	com := &scpCommand{}
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

func initRemoveUnitCommand(args ...string) (cmd.Command, error) {
	com := newRemoveUnitCommand()
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
