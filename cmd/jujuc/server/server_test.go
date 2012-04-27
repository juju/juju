package server_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/cmd/jujuc/server"
	"net/rpc"
	"os"
	"path/filepath"
)

type RpcCommand struct {
	Value string
}

func (c *RpcCommand) Info() *cmd.Info {
	return &cmd.Info{"magic", "", "do magic", "blah doc"}
}

func (c *RpcCommand) Init(f *gnuflag.FlagSet, args []string) error {
	f.StringVar(&c.Value, "value", "", "doc")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *RpcCommand) Run(ctx *cmd.Context) error {
	if c.Value != "zyxxy" {
		return errors.New("insufficiently magic")
	}
	ctx.Stdout.Write([]byte("eye of newt\n"))
	ctx.Stderr.Write([]byte("toe of frog\n"))
	return ioutil.WriteFile(ctx.AbsPath("local"), []byte{}, 0644)
}

func factory(contextId, cmdName string) (cmd.Command, error) {
	if contextId != "merlin" {
		return nil, errors.New("unknown client")
	}
	if cmdName != "magic" {
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
		"merlin", dir, "magic", []string{"--value", "zyxxy"}})
	c.Assert(err, IsNil)
	c.Assert(resp.Code, Equals, 0)
	c.Assert(string(resp.Stdout), Equals, "eye of newt\n")
	c.Assert(string(resp.Stderr), Equals, "toe of frog\n")
	_, err = os.Stat(filepath.Join(dir, "local"))
	c.Assert(err, IsNil)
}

func (s *ServerSuite) TestBadCommandName(c *C) {
	dir := c.MkDir()
	_, err := s.Call(c, server.Request{"merlin", dir, "", nil})
	c.Assert(err, ErrorMatches, "bad request: command not specified")
	_, err = s.Call(c, server.Request{"merlin", dir, "witchcraft", nil})
	c.Assert(err, ErrorMatches, `bad request: unknown command "witchcraft"`)
}

func (s *ServerSuite) TestBadDir(c *C) {
	for _, req := range []server.Request{
		{"merlin", "", "cmd", nil},
		{"merlin", "foo/bar", "cmd", nil},
	} {
		_, err := s.Call(c, req)
		c.Assert(err, ErrorMatches, "bad request: Dir is not absolute")
	}
}

func (s *ServerSuite) TestBadContextId(c *C) {
	_, err := s.Call(c, server.Request{"mordred", c.MkDir(), "magic", nil})
	c.Assert(err, ErrorMatches, "bad request: unknown client")
}

func (s *ServerSuite) AssertBadCommand(c *C, args []string, code int) server.Response {
	resp, err := s.Call(c, server.Request{"merlin", c.MkDir(), args[0], args[1:]})
	c.Assert(err, IsNil)
	c.Assert(resp.Code, Equals, code)
	return resp
}

func (s *ServerSuite) TestParseError(c *C) {
	resp := s.AssertBadCommand(c, []string{"magic", "--cheese"}, 2)
	c.Assert(string(resp.Stdout), Equals, "")
	c.Assert(string(resp.Stderr), Equals, `usage: magic [options]
purpose: do magic

options:
--value (= "")
    doc

blah doc
error: flag provided but not defined: --cheese
`)
}

func (s *ServerSuite) TestBrokenCommand(c *C) {
	resp := s.AssertBadCommand(c, []string{"magic"}, 1)
	c.Assert(string(resp.Stdout), Equals, "")
	c.Assert(string(resp.Stderr), Equals, "error: insufficiently magic\n")
}
