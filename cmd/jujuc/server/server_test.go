package server_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"net/rpc"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type RpcCommand struct {
	Value string
	Slow  bool
}

func (c *RpcCommand) Info() *cmd.Info {
	return &cmd.Info{"remote", "", "act at a distance", "blah doc"}
}

func (c *RpcCommand) Init(f *gnuflag.FlagSet, args []string) error {
	f.StringVar(&c.Value, "value", "", "doc")
	f.BoolVar(&c.Slow, "slow", false, "doc")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *RpcCommand) Run(ctx *cmd.Context) error {
	if c.Value == "error" {
		return errors.New("blam")
	}
	if c.Slow {
		<-time.After(50 * time.Millisecond)
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
	server   *server.Server
	sockPath string
	err      chan error
}

var _ = Suite(&ServerSuite{})

func (s *ServerSuite) SetUpTest(c *C) {
	s.sockPath = filepath.Join(c.MkDir(), "test.sock")
	srv, err := server.NewServer(factory, s.sockPath)
	c.Assert(err, IsNil)
	c.Assert(srv, NotNil)
	s.server = srv
	s.err = make(chan error)
	go func() { s.err <- s.server.Run() }()
}

func (s *ServerSuite) TearDownTest(c *C) {
	s.server.Close()
	c.Assert(<-s.err, IsNil)
	_, err := os.Open(s.sockPath)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *ServerSuite) Call(c *C, req server.Request) (resp server.Response, err error) {
	client, err := rpc.Dial("unix", s.sockPath)
	c.Assert(err, IsNil)
	defer client.Close()
	err = client.Call("Jujuc.Main", req, &resp)
	return resp, err
}

func (s *ServerSuite) TestHappyPath(c *C) {
	dir := c.MkDir()
	resp, err := s.Call(c, server.Request{
		"validCtx", dir, "remote", []string{"--value", "something"},
	})
	c.Assert(err, IsNil)
	c.Assert(resp.Code, Equals, 0)
	c.Assert(string(resp.Stdout), Equals, "eye of newt\n")
	c.Assert(string(resp.Stderr), Equals, "toe of frog\n")
	content, err := ioutil.ReadFile(filepath.Join(dir, "local"))
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "something")
}

func (s *ServerSuite) TestLocks(c *C) {
	var wg sync.WaitGroup
	t0 := time.Now()
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			dir := c.MkDir()
			resp, err := s.Call(c, server.Request{
				"validCtx", dir, "remote", []string{"--slow"},
			})
			c.Assert(err, IsNil)
			c.Assert(resp.Code, Equals, 0)
			wg.Done()
		}()
	}
	wg.Wait()
	t1 := time.Now()
	c.Assert(t0.Add(200*time.Millisecond).Before(t1), Equals, true)
}

func (s *ServerSuite) TestBadCommandName(c *C) {
	dir := c.MkDir()
	_, err := s.Call(c, server.Request{"validCtx", dir, "", nil})
	c.Assert(err, ErrorMatches, "bad request: command not specified")
	_, err = s.Call(c, server.Request{"validCtx", dir, "witchcraft", nil})
	c.Assert(err, ErrorMatches, `bad request: unknown command "witchcraft"`)
}

func (s *ServerSuite) TestBadDir(c *C) {
	for _, req := range []server.Request{
		{"validCtx", "", "anything", nil},
		{"validCtx", "foo/bar", "anything", nil},
	} {
		_, err := s.Call(c, req)
		c.Assert(err, ErrorMatches, "bad request: Dir is not absolute")
	}
}

func (s *ServerSuite) TestBadContextId(c *C) {
	_, err := s.Call(c, server.Request{"whatever", c.MkDir(), "remote", nil})
	c.Assert(err, ErrorMatches, `bad request: unknown context "whatever"`)
}

func (s *ServerSuite) AssertBadCommand(c *C, args []string, code int) server.Response {
	resp, err := s.Call(c, server.Request{"validCtx", c.MkDir(), args[0], args[1:]})
	c.Assert(err, IsNil)
	c.Assert(resp.Code, Equals, code)
	return resp
}

func (s *ServerSuite) TestParseError(c *C) {
	resp := s.AssertBadCommand(c, []string{"remote", "--cheese"}, 2)
	c.Assert(string(resp.Stdout), Equals, "")
	c.Assert(string(resp.Stderr), Equals, "error: flag provided but not defined: --cheese\n")
}

func (s *ServerSuite) TestBrokenCommand(c *C) {
	resp := s.AssertBadCommand(c, []string{"remote", "--value", "error"}, 1)
	c.Assert(string(resp.Stdout), Equals, "")
	c.Assert(string(resp.Stderr), Equals, "error: blam\n")
}
