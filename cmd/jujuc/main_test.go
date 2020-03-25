// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	stdtesting "testing"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/sockets"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

func TestPackage(t *stdtesting.T) {
	// TODO(waigani) 2014-03-19 bug 1294458
	// Refactor to use base suites

	coretesting.MgoSSLTestPackage(t)
}

type MainSuite struct{}

var _ = gc.Suite(&MainSuite{})

var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the jujud
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *flagRunMain {
		os.Exit(Main(flag.Args()))
	}
}

func checkMessage(c *gc.C, msg string, cmd ...string) {
	args := append([]string{"-test.run", "TestRunMain", "-run-main", "--", names.Jujuc}, cmd...)
	c.Logf("check %#v", args)
	ps := exec.Command(os.Args[0], args...)
	output, err := ps.CombinedOutput()
	c.Logf(string(output))
	c.Assert(err, gc.ErrorMatches, "exit status 2")
	lines := strings.Split(string(output), "\n")
	c.Assert(lines[len(lines)-2], jc.Contains, msg)
}

var expectedProviders = []string{
	"ec2",
	"maas",
	"openstack",
}

type RemoteCommand struct {
	cmd.CommandBase
	msg string
}

var expectUsage = `Usage: remote [options]

Summary:
test jujuc

Options:
--error (= "")
    if set, fail

Details:
here is some documentation
`

func (c *RemoteCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remote",
		Purpose: "test jujuc",
		Doc:     "here is some documentation",
	})
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

func run(c *gc.C, sockPath sockets.Socket, contextId string, exit int, stdin []byte, cmd ...string) string {
	args := append([]string{"-test.run", "TestRunMain", "-run-main", "--"}, cmd...)
	c.Logf("check %v %#v", os.Args[0], args)
	ps := exec.Command(os.Args[0], args...)
	ps.Stdin = bytes.NewBuffer(stdin)
	ps.Dir = c.MkDir()
	ps.Env = []string{
		fmt.Sprintf("JUJU_AGENT_SOCKET_ADDRESS=%s", sockPath.Address),
		fmt.Sprintf("JUJU_AGENT_SOCKET_NETWORK=%s", sockPath.Network),
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

type HookToolMainSuite struct {
	sockPath sockets.Socket
	server   *jujuc.Server
}

var _ = gc.Suite(&HookToolMainSuite{})

func osDependentSockPath(c *gc.C) sockets.Socket {
	sockPath := filepath.Join(c.MkDir(), "test.sock")
	if runtime.GOOS == "windows" {
		return sockets.Socket{Address: `\\.\pipe` + sockPath[2:]}
	}
	return sockets.Socket{Network: "unix", Address: sockPath}
}

func (s *HookToolMainSuite) SetUpSuite(c *gc.C) {
	loggo.DefaultContext().AddWriter("default", cmd.NewWarningWriter(os.Stderr))
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
	srv, err := jujuc.NewServer(factory, s.sockPath, "")
	c.Assert(err, jc.ErrorIsNil)
	s.server = srv
	go func() {
		if err := s.server.Run(); err != nil {
			c.Fatalf("server died: %s", err)
		}
	}()
}

func (s *HookToolMainSuite) TearDownSuite(c *gc.C) {
	s.server.Close()
}

var argsTests = []struct {
	args   []string
	code   int
	output string
}{
	{[]string{"remote"}, 0, "success!\n"},
	{[]string{"/path/to/remote"}, 0, "success!\n"},
	{[]string{"remote", "--help"}, 0, expectUsage},
	{[]string{"unknown"}, 1, "bad request: bad command: unknown\n"},
	{[]string{"remote", "--error", "borken"}, 1, "borken\n"},
	{[]string{"remote", "--unknown"}, 2, "option provided but not defined: --unknown\n"},
	{[]string{"remote", "unwanted"}, 2, `unrecognized args: ["unwanted"]` + "\n"},
}

func (s *HookToolMainSuite) TestArgs(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	for _, t := range argsTests {
		c.Log(t.args)
		output := run(c, s.sockPath, "bill", t.code, nil, t.args...)
		c.Assert(output, jc.Contains, t.output)
	}
}

func (s *HookToolMainSuite) TestNoClientId(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	output := run(c, s.sockPath, "", 1, nil, "remote")
	c.Assert(output, jc.Contains, "JUJU_CONTEXT_ID not set\n")
}

func (s *HookToolMainSuite) TestBadClientId(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	output := run(c, s.sockPath, "ben", 1, nil, "remote")
	c.Assert(output, jc.Contains, "bad request: bad context: ben\n")
}

func (s *HookToolMainSuite) TestNoSockPath(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	output := run(c, sockets.Socket{}, "bill", 1, nil, "remote")
	c.Assert(output, jc.Contains, "JUJU_AGENT_SOCKET_ADDRESS not set\n")
}

func (s *HookToolMainSuite) TestBadSockPath(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	badSock := filepath.Join(c.MkDir(), "bad.sock")
	output := run(c, sockets.Socket{Address: badSock, Network: "unix"}, "bill", 1, nil, "remote")
	err := fmt.Sprintf("^.* dial unix %s: .*\n", badSock)
	c.Assert(output, gc.Matches, err)
}

func (s *HookToolMainSuite) TestStdin(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: test panics on CryptAcquireContext on windows")
	}
	output := run(c, s.sockPath, "bill", 0, []byte("some standard input"), "remote")
	c.Assert(output, gc.Equals, "some standard input")
}
