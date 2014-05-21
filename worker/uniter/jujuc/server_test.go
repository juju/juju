// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/rpc"
	"os"
	"path/filepath"
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
	"launchpad.net/gnuflag"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/exec"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

type RpcCommand struct {
	cmd.CommandBase
	Value string
	Slow  bool
}

func (c *RpcCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remote",
		Purpose: "act at a distance",
		Doc:     "blah doc",
	}
}

func (c *RpcCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Value, "value", "", "doc")
	f.BoolVar(&c.Slow, "slow", false, "doc")
}

func (c *RpcCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *RpcCommand) Run(ctx *cmd.Context) error {
	if c.Value == "error" {
		return errors.New("blam")
	}
	if c.Slow {
		time.Sleep(testing.ShortWait)
		return nil
	}
	ctx.Stdout.Write([]byte("eye of newt\n"))
	ctx.Stderr.Write([]byte("toe of frog\n"))
	return ioutil.WriteFile(ctx.AbsPath("local"), []byte(c.Value), 0644)
}

func factory(contextId, cmdName string) (cmd.Command, error) {
	if contextId != "validCtx" {
		return nil, fmt.Errorf("unknown context %q", contextId)
	}
	if cmdName != "remote" {
		return nil, fmt.Errorf("unknown command %q", cmdName)
	}
	return &RpcCommand{}, nil
}

type ServerSuite struct {
	testing.BaseSuite
	server   *jujuc.Server
	sockPath string
	err      chan error
}

var _ = gc.Suite(&ServerSuite{})

func (s *ServerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.sockPath = filepath.Join(c.MkDir(), "test.sock")
	srv, err := jujuc.NewServer(factory, s.sockPath)
	c.Assert(err, gc.IsNil)
	c.Assert(srv, gc.NotNil)
	s.server = srv
	s.err = make(chan error)
	go func() { s.err <- s.server.Run() }()
}

func (s *ServerSuite) TearDownTest(c *gc.C) {
	s.server.Close()
	c.Assert(<-s.err, gc.IsNil)
	_, err := os.Open(s.sockPath)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	s.BaseSuite.TearDownTest(c)
}

func (s *ServerSuite) Call(c *gc.C, req jujuc.Request) (resp exec.ExecResponse, err error) {
	client, err := rpc.Dial("unix", s.sockPath)
	c.Assert(err, gc.IsNil)
	defer client.Close()
	err = client.Call("Jujuc.Main", req, &resp)
	return resp, err
}

func (s *ServerSuite) TestHappyPath(c *gc.C) {
	dir := c.MkDir()
	resp, err := s.Call(c, jujuc.Request{
		"validCtx", dir, "remote", []string{"--value", "something"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(resp.Code, gc.Equals, 0)
	c.Assert(string(resp.Stdout), gc.Equals, "eye of newt\n")
	c.Assert(string(resp.Stderr), gc.Equals, "toe of frog\n")
	content, err := ioutil.ReadFile(filepath.Join(dir, "local"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, "something")
}

func (s *ServerSuite) TestLocks(c *gc.C) {
	var wg sync.WaitGroup
	t0 := time.Now()
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			dir := c.MkDir()
			resp, err := s.Call(c, jujuc.Request{
				"validCtx", dir, "remote", []string{"--slow"},
			})
			c.Assert(err, gc.IsNil)
			c.Assert(resp.Code, gc.Equals, 0)
			wg.Done()
		}()
	}
	wg.Wait()
	t1 := time.Now()
	c.Assert(t0.Add(4*testing.ShortWait).Before(t1), gc.Equals, true)
}

func (s *ServerSuite) TestBadCommandName(c *gc.C) {
	dir := c.MkDir()
	_, err := s.Call(c, jujuc.Request{"validCtx", dir, "", nil})
	c.Assert(err, gc.ErrorMatches, "bad request: command not specified")
	_, err = s.Call(c, jujuc.Request{"validCtx", dir, "witchcraft", nil})
	c.Assert(err, gc.ErrorMatches, `bad request: unknown command "witchcraft"`)
}

func (s *ServerSuite) TestBadDir(c *gc.C) {
	for _, req := range []jujuc.Request{
		{"validCtx", "", "anything", nil},
		{"validCtx", "foo/bar", "anything", nil},
	} {
		_, err := s.Call(c, req)
		c.Assert(err, gc.ErrorMatches, "bad request: Dir is not absolute")
	}
}

func (s *ServerSuite) TestBadContextId(c *gc.C) {
	_, err := s.Call(c, jujuc.Request{"whatever", c.MkDir(), "remote", nil})
	c.Assert(err, gc.ErrorMatches, `bad request: unknown context "whatever"`)
}

func (s *ServerSuite) AssertBadCommand(c *gc.C, args []string, code int) exec.ExecResponse {
	resp, err := s.Call(c, jujuc.Request{"validCtx", c.MkDir(), args[0], args[1:]})
	c.Assert(err, gc.IsNil)
	c.Assert(resp.Code, gc.Equals, code)
	return resp
}

func (s *ServerSuite) TestParseError(c *gc.C) {
	resp := s.AssertBadCommand(c, []string{"remote", "--cheese"}, 2)
	c.Assert(string(resp.Stdout), gc.Equals, "")
	c.Assert(string(resp.Stderr), gc.Equals, "error: flag provided but not defined: --cheese\n")
}

func (s *ServerSuite) TestBrokenCommand(c *gc.C) {
	resp := s.AssertBadCommand(c, []string{"remote", "--value", "error"}, 1)
	c.Assert(string(resp.Stdout), gc.Equals, "")
	c.Assert(string(resp.Stderr), gc.Equals, "error: blam\n")
}

type NewCommandSuite struct {
	ContextSuite
}

var _ = gc.Suite(&NewCommandSuite{})

var newCommandTests = []struct {
	name string
	err  string
}{
	{"close-port", ""},
	{"config-get", ""},
	{"juju-log", ""},
	{"open-port", ""},
	{"relation-get", ""},
	{"relation-ids", ""},
	{"relation-list", ""},
	{"relation-set", ""},
	{"unit-get", ""},
	{"random", "unknown command: random"},
}

func (s *NewCommandSuite) TestNewCommand(c *gc.C) {
	ctx := s.GetHookContext(c, 0, "")
	for _, t := range newCommandTests {
		com, err := jujuc.NewCommand(ctx, t.name)
		if t.err == "" {
			// At this level, just check basic sanity; commands are tested in
			// more detail elsewhere.
			c.Assert(err, gc.IsNil)
			c.Assert(com.Info().Name, gc.Equals, t.name)
		} else {
			c.Assert(com, gc.IsNil)
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}
