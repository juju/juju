package main

import (
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/dummy"
	"launchpad.net/juju/go/testing"
	"os"
	"path/filepath"
	"reflect"
)

type cmdSuite struct {
	testing.LoggingSuite
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
	s.LoggingSuite.SetUpTest(c)
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

	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
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
var EnvironmentInitTests = []func() (cmd.Command, []string){
	func() (cmd.Command, []string) { return new(BootstrapCommand), nil },
	func() (cmd.Command, []string) { return new(DestroyCommand), nil },
	func() (cmd.Command, []string) {
		return new(DeployCommand), []string{"charm-name", "service-name"}
	},
}

// TestEnvironmentInit tests that all commands which accept
// the --environment variable initialise their
// environment name correctly.
func (*cmdSuite) TestEnvironmentInit(c *C) {
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

		com, args = cmdFunc()
		testInit(c, com, append(args, "hotdog"), "unrecognized args.*")
	}
}

func runCommand(com cmd.Command, args ...string) (opc chan dummy.Operation, errc chan error) {
	errc = make(chan error, 1)
	opc = make(chan dummy.Operation)
	dummy.Reset()
	dummy.Listen(opc)
	go func() {
		// signal that we're done with this ops channel.
		defer dummy.Listen(nil)

		err := com.Init(newFlagSet(), args)
		if err != nil {
			errc <- err
			return
		}

		err = com.Run(cmd.DefaultContext())
		errc <- err
	}()
	return
}

func op(kind dummy.OperationKind, name string) dummy.Operation {
	return dummy.Operation{
		Env:  name,
		Kind: kind,
	}
}

func (*cmdSuite) TestBootstrapCommand(c *C) {
	// normal bootstrap
	opc, errc := runCommand(new(BootstrapCommand))
	c.Check(<-opc, Equals, op(dummy.OpBootstrap, "peckham"))
	c.Check(<-errc, IsNil)

	// bootstrap with tool uploading - checking that a file
	// is uploaded should be sufficient, as the detailed semantics
	// of UploadTools are tested in environs.
	opc, errc = runCommand(new(BootstrapCommand), "--upload-tools")
	c.Check(<-opc, Equals, op(dummy.OpPutFile, "peckham"))
	c.Check(<-opc, Equals, op(dummy.OpBootstrap, "peckham"))
	c.Check(<-errc, IsNil)

	envs, err := environs.ReadEnvirons("")
	c.Assert(err, IsNil)
	env, err := envs.Open("peckham")
	c.Assert(err, IsNil)
	dir := c.MkDir()
	err = environs.GetTools(env, dir)
	c.Assert(err, IsNil)

	// bootstrap with broken environment
	opc, errc = runCommand(new(BootstrapCommand), "-e", "barking")
	c.Check((<-opc).Kind, Equals, dummy.OpNone)
	c.Check(<-errc, ErrorMatches, `broken environment`)
}

func (*cmdSuite) TestDestroyCommand(c *C) {
	// normal destroy
	opc, errc := runCommand(new(DestroyCommand))
	c.Check(<-opc, Equals, op(dummy.OpDestroy, "peckham"))
	c.Check(<-errc, IsNil)

	// destroy with broken environment
	opc, errc = runCommand(new(DestroyCommand), "-e", "barking")
	c.Check((<-opc).Kind, Equals, dummy.OpNone)
	c.Check(<-errc, ErrorMatches, `broken environment`)
}

func initDeployCommand(args ...string) (*DeployCommand, error) {
	com := &DeployCommand{}
	return com, com.Init(newFlagSet(), args)
}

func (*cmdSuite) TestDeployCommandInit(c *C) {
	os.Setenv("JUJU_REPOSITORY", "/path/to/repo")

	// missing args
	_, err := initDeployCommand()
	c.Assert(err, ErrorMatches, "no charm specified")

	// minimal
	com, err := initDeployCommand("charm-name")
	c.Assert(err, IsNil)
	c.Assert(com, DeepEquals, &DeployCommand{
		CharmName: "charm-name", NumUnits: 1, RepoPath: "/path/to/repo",
	})

	// with service name
	com, err = initDeployCommand("charm-name", "service-name")
	c.Assert(err, IsNil)
	c.Assert(com, DeepEquals, &DeployCommand{
		CharmName: "charm-name", ServiceName: "service-name", NumUnits: 1, RepoPath: "/path/to/repo",
	})

	// config
	com, err = initDeployCommand("--config", "/path/to/config.yaml", "charm-name")
	c.Assert(err, IsNil)
	c.Assert(com, DeepEquals, &DeployCommand{
		CharmName: "charm-name", NumUnits: 1, RepoPath: "/path/to/repo", ConfPath: "/path/to/config.yaml",
	})

	// repository
	com, err = initDeployCommand("--repository", "/path/to/another-repo", "charm-name")
	c.Assert(err, IsNil)
	c.Assert(com, DeepEquals, &DeployCommand{
		CharmName: "charm-name", NumUnits: 1, RepoPath: "/path/to/another-repo",
	})

	// upgrade
	for _, flag := range []string{"--upgrade", "-u"} {
		com, err = initDeployCommand("charm-name", flag)
		c.Assert(err, IsNil)
		c.Assert(com, DeepEquals, &DeployCommand{
			CharmName: "charm-name", NumUnits: 1, RepoPath: "/path/to/repo", Upgrade: true,
		})
	}

	// num-units
	for _, flag := range []string{"--num-units", "-n"} {
		com, err = initDeployCommand("charm-name", flag, "32")
		c.Assert(err, IsNil)
		c.Assert(com, DeepEquals, &DeployCommand{
			CharmName: "charm-name", NumUnits: 32, RepoPath: "/path/to/repo",
		})
	}
	_, err = initDeployCommand("charm-name", "--num-units", "0")
	c.Assert(err, ErrorMatches, "must deploy at least one unit")

	// environment tested elsewhere
}
