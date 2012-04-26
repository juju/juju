package main_test

import (
	"errors"
	"flag"
	"fmt"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujuc"
	"launchpad.net/juju/go/cmd/jujuc/server"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

var flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// Reentrancy point for testing (something as close as possible to) the jujuc
// tool itself.
func TestRunMain(t *testing.T) {
	if *flagRunMain {
		main.Main(flag.Args())
	}
}

type RemoteCommand struct {
	msg string
}

var expectUsage = `usage: (-> jujuc) remote [options]
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
		fmt.Println("BLAM", c.msg)
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
	factory := func(contextId string) ([]cmd.Command, error) {
		if contextId != "bill" {
			return nil, fmt.Errorf("bad context: %s", contextId)
		}
		return []cmd.Command{&RemoteCommand{}}, nil
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

func (s *MainSuite) TestHappyPath(c *C) {
	output := run(c, s.sockPath, "bill", 0, "remote")
	c.Assert(output, Equals, "success!\n")
}

func (s *MainSuite) TestBadRun(c *C) {
	output := run(c, s.sockPath, "bill", 1, "remote", "--error", "borken")
	c.Assert(output, Equals, "ERROR: borken\n")
}

func (s *MainSuite) TestBadFlag(c *C) {
	output := run(c, s.sockPath, "bill", 2, "remote", "--unknown")
	c.Assert(output, Equals, "ERROR: flag provided but not defined: --unknown\n"+expectUsage)
}

func (s *MainSuite) TestBadArg(c *C) {
	output := run(c, s.sockPath, "bill", 2, "remote", "unwanted")
	c.Assert(output, Equals, "ERROR: unrecognised args: [unwanted]\n"+expectUsage)
}

func (s *MainSuite) TestBadCommand(c *C) {
	output := run(c, s.sockPath, "bill", 2, "unknown")
	lines := strings.Split(output, "\n")
	c.Assert(lines[:3], DeepEquals, []string{
		"ERROR: unrecognised command: (-> jujuc) unknown",
		"usage: (-> jujuc) <command> ...",
		"purpose: invoke a hosted command inside the unit agent process",
	})
	c.Assert(lines[len(lines)-3:], DeepEquals, []string{
		"commands:",
		"    remote  test jujuc",
		"",
	})
}

func AssertOutput(c *C, actual, expected string) {
	c.Assert(actual, Matches, expected+server.JUJUC_DOC)
}

func (s *MainSuite) TestNoSockPath(c *C) {
	output := run(c, "", "bill", 1, "remote")
	AssertOutput(c, output, "FATAL: JUJU_AGENT_SOCKET not set\n")
}

func (s *MainSuite) TestBadSockPath(c *C) {
	output := run(c, filepath.Join(c.MkDir(), "bad.sock"), "bill", 1, "remote")
	AssertOutput(c, output, "FATAL: dial unix .*: no such file or directory\n")
}

func (s *MainSuite) TestNoClientId(c *C) {
	output := run(c, s.sockPath, "", 1, "remote")
	AssertOutput(c, output, "FATAL: JUJU_CONTEXT_ID not set\n")
}

func (s *MainSuite) TestBadClientId(c *C) {
	output := run(c, s.sockPath, "ben", 1, "remote")
	AssertOutput(c, output, "FATAL: bad request: bad context: ben\n")
}
