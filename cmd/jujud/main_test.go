// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	stdtesting "testing"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/gnuflag"

	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/names"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var caCertFile string

func mkdtemp(prefix string) string {
	d, err := ioutil.TempDir("", prefix)
	if err != nil {
		panic(err)
	}
	return d
}

func mktemp(prefix string, content string) string {
	f, err := ioutil.TempFile("", prefix)
	if err != nil {
		panic(err)
	}
	_, err = f.WriteString(content)
	if err != nil {
		panic(err)
	}
	f.Close()
	return f.Name()
}

func TestPackage(t *stdtesting.T) {
	// TODO(waigani) 2014-03-19 bug 1294458
	// Refactor to use base suites

	// Change the path to "juju-run", so that the
	// tests don't try to write to /usr/local/bin.
	agentcmd.JujuRun = mktemp("juju-run", "")
	defer os.Remove(agentcmd.JujuRun)

	// Create a CA certificate available for all tests.
	caCertFile = mktemp("juju-test-cert", coretesting.CACert)
	defer os.Remove(caCertFile)

	coretesting.MgoTestPackage(t)
}

type MainSuite struct{}

var _ = gc.Suite(&MainSuite{})

var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the jujud
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *flagRunMain {
		MainWrapper(flag.Args())
	}
}

func checkMessage(c *gc.C, msg string, cmd ...string) {
	args := append([]string{"-test.run", "TestRunMain", "-run-main", "--", names.Jujud}, cmd...)
	c.Logf("check %#v", args)
	ps := exec.Command(os.Args[0], args...)
	output, err := ps.CombinedOutput()
	c.Logf(string(output))
	c.Assert(err, gc.ErrorMatches, "exit status 2")
	lines := strings.Split(string(output), "\n")
	c.Assert(lines[len(lines)-2], gc.Equals, "error: "+msg)
}

func (s *MainSuite) TestParseErrors(c *gc.C) {
	// Check all the obvious parse errors
	checkMessage(c, "unrecognized command: jujud cavitate", "cavitate")
	msgf := "flag provided but not defined: --cheese"
	checkMessage(c, msgf, "--cheese", "cavitate")

	cmds := []string{"bootstrap-state", "unit", "machine"}
	for _, cmd := range cmds {
		checkMessage(c, msgf, cmd, "--cheese")
	}

	msga := `unrecognized args: ["toastie"]`
	checkMessage(c, msga,
		"bootstrap-state",
		"--env-config", b64yaml{"blah": "blah"}.encode(),
		"--instance-id", "inst",
		"toastie")
	checkMessage(c, msga, "unit",
		"--unit-name", "un/0",
		"toastie")
	checkMessage(c, msga, "machine",
		"--machine-id", "42",
		"toastie")
}

var expectedProviders = []string{
	"ec2",
	"maas",
	"openstack",
}

func (s *MainSuite) TestProvidersAreRegistered(c *gc.C) {
	// check that all the expected providers are registered
	for _, name := range expectedProviders {
		_, err := environs.Provider(name)
		c.Assert(err, jc.ErrorIsNil)
	}
}

type RemoteCommand struct {
	cmd.CommandBase
	msg string
}

var expectUsage = `usage: remote [options]
purpose: test jujuc

options:
--error (= "")
    if set, fail

here is some documentation
`

func (c *RemoteCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remote",
		Purpose: "test jujuc",
		Doc:     "here is some documentation",
	}
}

func (c *RemoteCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.msg, "error", "", "if set, fail")
}

func (c *RemoteCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *RemoteCommand) Run(ctx *cmd.Context) error {
	if c.msg != "" {
		return errors.New(c.msg)
	}
	n, err := io.Copy(ctx.Stdout, ctx.Stdin)
	if err != nil {
		return err
	}
	if n == 0 {
		fmt.Fprintf(ctx.Stdout, "success!\n")
	}
	return nil
}

func run(c *gc.C, sockPath string, contextId string, exit int, stdin []byte, cmd ...string) string {
	args := append([]string{"-test.run", "TestRunMain", "-run-main", "--"}, cmd...)
	c.Logf("check %v %#v", os.Args[0], args)
	ps := exec.Command(os.Args[0], args...)
	ps.Stdin = bytes.NewBuffer(stdin)
	ps.Dir = c.MkDir()
	ps.Env = []string{
		fmt.Sprintf("JUJU_AGENT_SOCKET=%s", sockPath),
		fmt.Sprintf("JUJU_CONTEXT_ID=%s", contextId),
		// Code that imports github.com/juju/juju/testing needs to
		// be able to find that module at runtime (via build.Import),
		// so we have to preserve that env variable.
		os.ExpandEnv("GOPATH=${GOPATH}"),
	}
	output, err := ps.CombinedOutput()
	if exit == 0 {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}

type JujuCMainSuite struct {
	sockPath string
	server   *jujuc.Server
}

var _ = gc.Suite(&JujuCMainSuite{})

func osDependentSockPath(c *gc.C) string {
	sockPath := filepath.Join(c.MkDir(), "test.sock")
	if runtime.GOOS == "windows" {
		return `\\.\pipe` + sockPath[2:]
	}
	return sockPath
}

func (s *JujuCMainSuite) SetUpSuite(c *gc.C) {
	factory := func(contextId, cmdName string) (cmd.Command, error) {
		if contextId != "bill" {
			return nil, fmt.Errorf("bad context: %s", contextId)
		}
		if cmdName != "remote" {
			return nil, fmt.Errorf("bad command: %s", cmdName)
		}
		return &RemoteCommand{}, nil
	}
	s.sockPath = osDependentSockPath(c)
	srv, err := jujuc.NewServer(factory, s.sockPath)
	c.Assert(err, jc.ErrorIsNil)
	s.server = srv
	go func() {
		if err := s.server.Run(); err != nil {
			c.Fatalf("server died: %s", err)
		}
	}()
}

func (s *JujuCMainSuite) TearDownSuite(c *gc.C) {
	s.server.Close()
}

var argsTests = []struct {
	args   []string
	code   int
	output string
}{
	{[]string{"jujuc", "whatever"}, 2, jujudDoc + "error: jujuc should not be called directly\n"},
	{[]string{"remote"}, 0, "success!\n"},
	{[]string{"/path/to/remote"}, 0, "success!\n"},
	{[]string{"remote", "--help"}, 0, expectUsage},
	{[]string{"unknown"}, 1, "error: bad request: bad command: unknown\n"},
	{[]string{"remote", "--error", "borken"}, 1, "error: borken\n"},
	{[]string{"remote", "--unknown"}, 2, "error: flag provided but not defined: --unknown\n"},
	{[]string{"remote", "unwanted"}, 2, `error: unrecognized args: ["unwanted"]` + "\n"},
}

func (s *JujuCMainSuite) TestArgs(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	for _, t := range argsTests {
		c.Log(t.args)
		output := run(c, s.sockPath, "bill", t.code, nil, t.args...)
		c.Assert(output, gc.Equals, t.output)
	}
}

func (s *JujuCMainSuite) TestNoClientId(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	output := run(c, s.sockPath, "", 1, nil, "remote")
	c.Assert(output, gc.Equals, "error: JUJU_CONTEXT_ID not set\n")
}

func (s *JujuCMainSuite) TestBadClientId(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	output := run(c, s.sockPath, "ben", 1, nil, "remote")
	c.Assert(output, gc.Equals, "error: bad request: bad context: ben\n")
}

func (s *JujuCMainSuite) TestNoSockPath(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	output := run(c, "", "bill", 1, nil, "remote")
	c.Assert(output, gc.Equals, "error: JUJU_AGENT_SOCKET not set\n")
}

func (s *JujuCMainSuite) TestBadSockPath(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	badSock := filepath.Join(c.MkDir(), "bad.sock")
	output := run(c, badSock, "bill", 1, nil, "remote")
	err := fmt.Sprintf("error: dial unix %s: .*\n", badSock)
	c.Assert(output, gc.Matches, err)
}

func (s *JujuCMainSuite) TestStdin(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	output := run(c, s.sockPath, "bill", 0, []byte("some standard input"), "remote")
	c.Assert(output, gc.Equals, "some standard input")
}
