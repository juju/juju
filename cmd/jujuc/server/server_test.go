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
	"strings"
)

type RpcCommand struct {
	Value string
}

func (c *RpcCommand) Info() *cmd.Info {
	return &cmd.Info{"magic", "[options]", "do magic", "blah doc"}
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

func factory(contextId string) ([]cmd.Command, error) {
	if contextId != "merlin" {
		return nil, errors.New("unknown client")
	}
	return []cmd.Command{&RpcCommand{}}, nil
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
	fmt.Println("calling", req.Args)
	client, err := rpc.Dial("unix", s.sockPath)
	c.Assert(err, IsNil)
	defer client.Close()
	err = client.Call("Jujuc.Main", req, &resp)
	return resp, err
}

func (s *ServerSuite) TestHappyPath(c *C) {
	dir := c.MkDir()
	resp, err := s.Call(c, server.Request{
		"merlin", dir, []string{"magic", "--value", "zyxxy"}})
	c.Assert(err, IsNil)
	c.Assert(resp.Code, Equals, 0)
	c.Assert(resp.Stdout, Equals, "eye of newt\n")
	c.Assert(resp.Stderr, Equals, "toe of frog\n")
	_, err = os.Stat(filepath.Join(dir, "local"))
	c.Assert(err, IsNil)
}

func (s *ServerSuite) TestBadArgs(c *C) {
	dir := c.MkDir()
	for _, req := range []server.Request{
		{"merlin", dir, nil},
		{"mordred", dir, nil},
	} {
		_, err := s.Call(c, req)
		c.Assert(err, ErrorMatches, "bad request: Args is too short")
	}
}

func (s *ServerSuite) TestBadDir(c *C) {
	for _, req := range []server.Request{
		{"merlin", "", []string{"cmd"}},
		{"merlin", "foo/bar", []string{"cmd"}},
	} {
		_, err := s.Call(c, req)
		c.Assert(err, ErrorMatches, "bad request: Dir is not absolute")
	}
}

func (s *ServerSuite) TestBadContextId(c *C) {
	_, err := s.Call(c, server.Request{"mordred", c.MkDir(), []string{"magic"}})
	c.Assert(err, ErrorMatches, "bad request: unknown client")
}

func (s *ServerSuite) AssertBadCommand(c *C, args []string, code int) server.Response {
	resp, err := s.Call(c, server.Request{"merlin", c.MkDir(), args})
	c.Assert(err, IsNil)
	c.Assert(resp.Code, Equals, code)
	return resp
}

func lines(s string) []string {
	return strings.Split(s, "\n")
}

func (s *ServerSuite) TestUnknownCommand(c *C) {
	resp := s.AssertBadCommand(c, []string{"witchcraft"}, 2)
	c.Assert(resp.Stdout, Equals, "")
	usageStart := []string{
		"ERROR: unrecognised command: (-> jujuc) witchcraft",
		"usage: (-> jujuc) <command> ...",
		"purpose: invoke a hosted command inside the unit agent process",
	}
	c.Assert(lines(resp.Stderr)[:3], DeepEquals, usageStart)
}

func (s *ServerSuite) TestParseError(c *C) {
	resp := s.AssertBadCommand(c, []string{"magic", "--cheese"}, 2)
	c.Assert(resp.Stdout, Equals, "")
	c.Assert(lines(resp.Stderr)[0], Equals, "ERROR: flag provided but not defined: --cheese")
}

func (s *ServerSuite) TestBrokenCommand(c *C) {
	resp := s.AssertBadCommand(c, []string{"magic"}, 1)
	c.Assert(resp.Stdout, Equals, "")
	c.Assert(lines(resp.Stderr)[0], Equals, "ERROR: insufficiently magic")
}
