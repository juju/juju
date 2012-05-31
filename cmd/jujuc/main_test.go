package main

import (
	"errors"
	"flag"
	"fmt"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/cmd/jujuc/server"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the jujuc
// tool itself.
func TestRunMain(t *testing.T) {
	if *flagRunMain {
		code, err := Main(flag.Args())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(code)
	}
}

type RemoteCommand struct {
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
		"remote", "", "test jujuc", "here is some documentation"}
}

func (c *RemoteCommand) Init(f *gnuflag.FlagSet, args []string) error {
	f.StringVar(&c.msg, "error", "", "if set, fail")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *RemoteCommand) Run(ctx *cmd.Context) error {
	if c.msg != "" {
		return errors.New(c.msg)
	}
	fmt.Fprintf(ctx.Stdout, "success!\n")
	return nil
}

func run(c *C, sockPath string, contextId string, exit int, cmd ...string) string {
	args := append([]string{"-test.run", "TestRunMain", "-run-main", "--"}, cmd...)
	ps := exec.Command(os.Args[0], args...)
	ps.Dir = c.MkDir()
	ps.Env = []string{
		fmt.Sprintf("JUJU_AGENT_SOCKET=%s", sockPath),
		fmt.Sprintf("JUJU_CONTEXT_ID=%s", contextId),
	}
	output, err := ps.CombinedOutput()
	if exit == 0 {
		c.Assert(err, IsNil)
	} else {
		c.Assert(err, ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}

type MainSuite struct {
	sockPath string
	server   *server.Server
}

var _ = Suite(&MainSuite{})

func (s *MainSuite) SetUpSuite(c *C) {
	factory := func(contextId, cmdName string) (cmd.Command, error) {
		if contextId != "bill" {
			return nil, fmt.Errorf("bad context: %s", contextId)
		}
		if cmdName != "remote" {
			return nil, fmt.Errorf("bad command: %s", cmdName)
		}
		return &RemoteCommand{}, nil
	}
	s.sockPath = filepath.Join(c.MkDir(), "test.sock")
	srv, err := server.NewServer(factory, s.sockPath)
	c.Assert(err, IsNil)
	s.server = srv
	go func() {
		if err := s.server.Run(); err != nil {
			c.Fatalf("server died: %s", err)
		}
	}()
}

func (s *MainSuite) TearDownSuite(c *C) {
	s.server.Close()
}

var argsTests = []struct {
	args   []string
	code   int
	output string
}{
	{[]string{"jujuc", "whatever"}, 2, Help + "error: jujuc should not be called directly\n"},
	{[]string{"remote"}, 0, "success!\n"},
	{[]string{"/path/to/remote"}, 0, "success!\n"},
	{[]string{"unknown"}, 1, "error: bad request: bad command: unknown\n"},
	{[]string{"remote", "--error", "borken"}, 1, "error: borken\n"},
	{[]string{"remote", "--unknown"}, 2, expectUsage + "error: flag provided but not defined: --unknown\n"},
	{[]string{"remote", "unwanted"}, 2, expectUsage + `error: unrecognized args: ["unwanted"]` + "\n"},
}

func (s *MainSuite) TestArgs(c *C) {
	for _, t := range argsTests {
		fmt.Println(t.args)
		output := run(c, s.sockPath, "bill", t.code, t.args...)
		c.Assert(output, Equals, t.output)
	}
}

func (s *MainSuite) TestNoClientId(c *C) {
	output := run(c, s.sockPath, "", 1, "remote")
	c.Assert(output, Equals, "error: JUJU_CONTEXT_ID not set\n")
}

func (s *MainSuite) TestBadClientId(c *C) {
	output := run(c, s.sockPath, "ben", 1, "remote")
	c.Assert(output, Equals, "error: bad request: bad context: ben\n")
}

func (s *MainSuite) TestNoSockPath(c *C) {
	output := run(c, "", "bill", 1, "remote")
	c.Assert(output, Equals, "error: JUJU_AGENT_SOCKET not set\n")
}

func (s *MainSuite) TestBadSockPath(c *C) {
	badSock := filepath.Join(c.MkDir(), "bad.sock")
	output := run(c, badSock, "bill", 1, "remote")
	err := fmt.Sprintf("error: dial unix %s: .*\n", badSock)
	c.Assert(output, Matches, err)
}
