// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspect_test

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	stdtesting "testing"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent/config"
	"github.com/juju/juju/agent/introspect"
	"github.com/juju/juju/internal/testing"
)

type IntrospectCommandSuite struct {
	testing.BaseSuite
}

func (s *IntrospectCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&config.DataDir, c.MkDir())
}

var _ = gc.Suite(&IntrospectCommandSuite{})

func (s *IntrospectCommandSuite) TestInitErrors(c *gc.C) {
	s.assertInitError(c, "either a query path or a --listen address must be specified")
	s.assertInitError(c, "a query path may not be specified with --listen", "query-path", "--listen=foo")
	s.assertInitError(c, `unrecognized args: \["path"\]`, "query", "path")
	s.assertInitError(c, "form value missing '='", "--post", "query-path", "foo")
}

func (*IntrospectCommandSuite) assertInitError(c *gc.C, expect string, args ...string) {
	err := cmdtesting.InitCommand(&introspect.IntrospectCommand{}, args)
	c.Assert(err, gc.ErrorMatches, expect)
}

func (*IntrospectCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, &introspect.IntrospectCommand{}, args...)
}

func (s *IntrospectCommandSuite) TestAutoDetectMachineAgent(c *gc.C) {
	machineDir := filepath.Join(config.DataDir, "agents", "machine-1024")
	err := os.MkdirAll(machineDir, 0755)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "query")
	c.Assert(err, gc.ErrorMatches, ".*machine-1024.*")
}

func (s *IntrospectCommandSuite) TestAutoDetectMachineAgentFails(c *gc.C) {
	machineDir := filepath.Join(config.DataDir, "agents")
	err := os.MkdirAll(machineDir, 0755)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "query")
	c.Assert(err, gc.ErrorMatches, "could not determine machine or controller agent tag")
}

func (s *IntrospectCommandSuite) TestAgentSpecified(c *gc.C) {
	_, err := s.run(c, "query", "--agent=unit-foo-0")
	c.Assert(err, gc.ErrorMatches, ".*unit-foo-0.*")
}

func (s *IntrospectCommandSuite) TestQuery(c *gc.C) {
	agentDir := filepath.Join(config.DataDir, "agents", "machine-0")
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	listener, err := net.Listen("unix", filepath.Join(agentDir, "introspection.socket"))
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
	agentDir := filepath.Join(config.DataDir, "agents", "machine-0")
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	listener, err := net.Listen("unix", filepath.Join(agentDir, "introspection.socket"))
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()

	srv := newServer(listener)
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	ctx, err := s.run(c, "missing", "--agent=machine-0")
	c.Assert(err.Error(), gc.Equals, "response returned 404 (Not Found)")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "404 page not found\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")

	ctx, err = s.run(c, "badness", "--agent=machine-0")
	c.Assert(err.Error(), gc.Equals, "response returned 500 (Internal Server Error)")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "argh\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *IntrospectCommandSuite) TestGetToPostEndpoint(c *gc.C) {
	agentDir := filepath.Join(config.DataDir, "agents", "machine-0")
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	listener, err := net.Listen("unix", filepath.Join(agentDir, "introspection.socket"))
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()

	srv := newServer(listener)
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	ctx, err := s.run(c, "postonly", "--agent=machine-0")
	c.Assert(err, gc.ErrorMatches, `response returned 405 \(Method Not Allowed\)`)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "postonly requires a POST request\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *IntrospectCommandSuite) TestPost(c *gc.C) {
	agentDir := filepath.Join(config.DataDir, "agents", "machine-0")
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	listener, err := net.Listen("unix", filepath.Join(agentDir, "introspection.socket"))
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()

	srv := newServer(listener)
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	ctx, err := s.run(c, "--post", "postonly", "--agent=machine-0", "single=value", "double=foo", "double=bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
double="foo"
double="bar"
single="value"
`[1:])
}

func (s *IntrospectCommandSuite) TestListen(c *gc.C) {
	agentDir := filepath.Join(config.DataDir, "agents", "machine-0")
	err := os.MkdirAll(agentDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	socketName := filepath.Join(agentDir, "introspection.socket")
	listener, err := net.Listen("unix", socketName)
	c.Assert(err, jc.ErrorIsNil)
	defer listener.Close()

	srv := newServer(listener)
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-run-listen="+config.DataDir)
	stderr, err := cmd.StderrPipe()
	c.Assert(err, jc.ErrorIsNil)
	defer stderr.Close()
	err = cmd.Start()
	c.Assert(err, jc.ErrorIsNil)

	scanner := bufio.NewScanner(stderr)
	c.Assert(scanner.Scan(), jc.IsTrue)
	line := scanner.Text()
	c.Assert(line, gc.Matches, "Exposing .* introspection socket on 127.0.0.1:.*")

	fields := strings.Fields(line)
	addr := fields[len(fields)-1]
	resp, err := http.Get(fmt.Sprintf("http://%s/query", addr))
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	body, err := io.ReadAll(resp.Body)
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
	mux.HandleFunc("/postonly", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("postonly requires a POST request\n"))
			return
		}
		_ = r.ParseForm()
		var keys []string
		for key := range r.Form {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			values := r.Form[key]
			for _, v := range values {
				fmt.Fprintf(w, "%s=%q\n", key, v)
			}
		}
	})
	srv := &http.Server{}
	srv.Handler = mux
	return srv
}

var flagListen = flag.String("run-listen", "", "DataDir of the Unix socket to connect the introspect command to using --listen=:0")

func TestRunListen(t *stdtesting.T) {
	if *flagListen != "" {
		introspectCommand := &introspect.IntrospectCommand{}
		args := append(flag.Args(), "--data-dir="+*flagListen, "--listen=127.0.0.1:0", "--agent=machine-0")
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
