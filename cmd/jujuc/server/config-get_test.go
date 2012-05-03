package server_test

import (
	"bytes"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/charm"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/cmd/jujuc/server"
	"launchpad.net/juju/go/state"
	"launchpad.net/juju/go/testing"
	"net/url"
)

func addDummyCharm(c *C, st *state.State) *state.Charm {
	ch := testing.Charms.Dir("dummy")
	u := fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL(u)
	burl, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := st.AddCharm(ch, curl, burl)
	c.Assert(err, IsNil)
	return dummy
}

func dummyContext(c *C) *cmd.Context {
	return &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}}
}

func str(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

type ConfigGetSuite struct {
	ctx *server.Context
}

var _ = Suite(&ConfigGetSuite{})

func (s *ConfigGetSuite) SetUpTest(c *C) {
	st, err := state.Initialize(&state.Info{
		Addrs: []string{zkAddr},
	})
	c.Assert(err, IsNil)
	s.ctx = &server.Context{
		Id:            "TestCtx",
		State:         st,
		LocalUnitName: "minecraft/0",
	}
	dummy := addDummyCharm(c, st)
	service, err := st.AddService("minecraft", dummy)
	c.Assert(err, IsNil)
	_, err = service.AddUnit()
	c.Assert(err, IsNil)
	conf, err := service.Config()
	c.Assert(err, IsNil)
	conf.Update(map[string]interface{}{
		"monsters":            false,
		"spline-reticulation": 45.0,
	})
	_, err = conf.Write()
	c.Assert(err, IsNil)
}

func (s *ConfigGetSuite) TearDownTest(c *C) {
	zk, session, err := zookeeper.Dial(zkAddr, 15e9)
	c.Assert(err, IsNil)
	event := <-session
	c.Assert(event.Ok(), Equals, true)
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)
	testing.ZkRemoveTree(zk, "/")
}

var configGetTests = []struct {
	args []string
	out  string
}{
	{[]string{"monsters"}, "false\n"},
	{[]string{"spline-reticulation"}, "45\n"},
	{[]string{"missing"}, "<nil>\n"},
	{nil, `map\[(spline-reticulation:45 monsters:false|monsters:false spline-reticulation:45)\]` + "\n"},
}

func (s *ConfigGetSuite) TestOutput(c *C) {
	for _, t := range configGetTests {
		com, err := s.ctx.GetCommand("config-get")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, 0)
		c.Assert(str(ctx.Stderr), Equals, "")
		c.Assert(str(ctx.Stdout), Matches, t.out)
	}
}

func (s *ConfigGetSuite) TestUnknownArg(c *C) {
	com, err := s.ctx.GetCommand("config-get")
	c.Assert(err, IsNil)
	err = com.Init(dummyFlagSet(), []string{"multiple", "keys"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["keys"\]`)
}

func (s *ConfigGetSuite) TestBadState(c *C) {
	s.ctx.State = nil
	com, err := s.ctx.GetCommand("config-get")
	c.Assert(com, IsNil)
	c.Assert(err, ErrorMatches, "context TestCtx cannot access state")
}

func (s *ConfigGetSuite) TestBadUnit(c *C) {
	s.ctx.LocalUnitName = ""
	com, err := s.ctx.GetCommand("config-get")
	c.Assert(com, IsNil)
	c.Assert(err, ErrorMatches, "context TestCtx is not attached to a unit")
}
