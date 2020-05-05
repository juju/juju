// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspect_test

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	stdtesting "testing"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/introspect"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/testing"
)

type IntrospectCommandSuite struct {
	testing.BaseSuite
}

func (s *IntrospectCommandSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("introspection socket does not run on windows")
	}
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&cmdutil.DataDir, c.MkDir())
}

var _ = gc.Suite(&IntrospectCommandSuite{})

func (s *IntrospectCommandSuite) TestInitErrors(c *gc.C) {
	s.assertInitError(c, "either a query path or a --listen address must be specified")
	s.assertInitError(c, "a query path may not be specified with --listen", "query-path", "--listen=foo")
	s.assertInitError(c, `unrecognized args: \["path"\]`, "query", "path")
}

func (*IntrospectCommandSuite) assertInitError(c *gc.C, expect string, args ...string) {
	err := cmdtesting.InitCommand(&introspect.IntrospectCommand{}, args)
	c.Assert(err, gc.ErrorMatches, expect)
}

func (*IntrospectCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, &introspect.IntrospectCommand{
		IntrospectionSocketName: func(tag names.Tag) string {
			return filepath.Join(cmdutil.DataDir, "jujud-"+tag.String())
		},
	}, args...)
}

func (s *IntrospectCommandSuite) TestAutoDetectMachineAgent(c *gc.C) {
	machineDir := filepath.Join(cmdutil.DataDir, "agents", "machine-1024")
	err := os.MkdirAll(machineDir, 0755)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "query")
	c.Assert(err, gc.ErrorMatches, ".*jujud-machine-1024.*")
}

func (s *IntrospectCommandSuite) TestAutoDetectMachineAgentFails(c *gc.C) {
	machineDir := filepath.Join(cmdutil.DataDir, "agents")
	err := os.MkdirAll(machineDir, 0755)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "query")
	c.Assert(err, gc.ErrorMatches, "could not determine machine or controller agent tag")
}

func (s *IntrospectCommandSuite) TestAgentSpecified(c *gc.C) {
	_, err := s.run(c, "query", "--agent=unit-foo-0")
	c.Assert(err, gc.ErrorMatches, ".*jujud-unit-foo-0.*")
}

func (s *IntrospectCommandSuite) TestQuery(c *gc.C) {
	listener, err := net.Listen("unix", "@"+filepath.Join(cmdutil.DataDir, "jujud-machine-0"))
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()

	srv := newServer(listener)
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	ctx, err := s.run(c, "query", "--agent=machine-0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "hello")
}

func (s *IntrospectCommandSuite) TestQueryFails(c *gc.C) {
	listener, err := net.Listen("unix", "@"+filepath.Join(cmdutil.DataDir, "jujud-machine-0"))
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()

	srv := newServer(listener)
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	ctx, err := s.run(c, "missing", "--agent=machine-0")
	c.Assert(err, gc.ErrorMatches, `response returned 404 \(Not Found\)`)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, fmt.Sprintf(`
Querying @%s introspection socket: missing
404 page not found
`[1:], filepath.Join(cmdutil.DataDir, "jujud-machine-0")))
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")

	ctx, err = s.run(c, "badness", "--agent=machine-0")
	c.Assert(err, gc.ErrorMatches, `response returned 500 \(Internal Server Error\)`)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, fmt.Sprintf(`
Querying @%s introspection socket: badness
argh
`[1:], filepath.Join(cmdutil.DataDir, "jujud-machine-0")))
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *IntrospectCommandSuite) TestListen(c *gc.C) {
	socketName := filepath.Join(cmdutil.DataDir, "jujud-machine-0")
	listener, err := net.Listen("unix", "@"+socketName)
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()

	srv := newServer(listener)
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-run-listen="+socketName)
	stderr, err := cmd.StderrPipe()
	c.Assert(err, jc.ErrorIsNil)
	defer stderr.Close()
	err = cmd.Start()
	c.Assert(err, jc.ErrorIsNil)

	scanner := bufio.NewScanner(stderr)
	c.Assert(scanner.Scan(), jc.IsTrue)
	line := scanner.Text()
	c.Assert(line, gc.Matches, "Exposing @.* introspection socket on 127.0.0.1:.*")

	fields := strings.Fields(line)
	addr := fields[len(fields)-1]
	resp, err := http.Get(fmt.Sprintf("http://%s/query", addr))
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(body), gc.Equals, "hello")
}

func newServer(l net.Listener) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	mux.HandleFunc("/badness", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "argh", http.StatusInternalServerError)
	})
	srv := &http.Server{}
	srv.Handler = mux
	return srv
}

var flagListen = flag.String("run-listen", "", "Name of the Unix socket to connect the introspect command to using --listen=:0")

func TestRunListen(t *stdtesting.T) {
	if *flagListen != "" {
		introspectCommand := &introspect.IntrospectCommand{
			IntrospectionSocketName: func(names.Tag) string {
				return *flagListen
			},
		}
		args := append(flag.Args(), "--listen=127.0.0.1:0", "--agent=machine-0")
		if err := cmdtesting.InitCommand(introspectCommand, args); err != nil {
			t.Fatal(err)
		}
		ctx, err := cmd.DefaultContext()
		if err != nil {
			t.Fatal(err)
		}
		if err := introspectCommand.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
