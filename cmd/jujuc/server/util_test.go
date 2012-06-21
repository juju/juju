package server_test

import (
	"bytes"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"net/url"
	stdtesting "testing"
)

var zkAddr string

func TestPackage(t *stdtesting.T) {
	srv := testing.StartZkServer()
	defer srv.Destroy()
	var err error
	zkAddr, err = srv.Addr()
	if err != nil {
		t.Fatalf("could not get ZooKeeper server address")
	}
	TestingT(t)
}

func addDummyCharm(c *C, st *state.State) *state.Charm {
	ch := testing.Charms.Dir("dummy")
	u := fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL(u)
	burl, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := st.AddCharm(ch, curl, burl, "dummy-sha256")
	c.Assert(err, IsNil)
	return dummy
}

func dummyContext(c *C) *cmd.Context {
	return &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}}
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

type UnitFixture struct {
	ctx     *server.ClientContext
	service *state.Service
	unit    *state.Unit
}

func (f *UnitFixture) SetUpTest(c *C) {
	st, err := state.Initialize(&state.Info{
		Addrs: []string{zkAddr},
	})
	c.Assert(err, IsNil)
	f.ctx = &server.ClientContext{
		Id:            "TestCtx",
		State:         st,
		LocalUnitName: "minecraft/0",
	}
	dummy := addDummyCharm(c, st)
	f.service, err = st.AddService("minecraft", dummy)
	c.Assert(err, IsNil)
	f.unit, err = f.service.AddUnit()
	c.Assert(err, IsNil)
}

func (f *UnitFixture) TearDownTest(c *C) {
	zk, session, err := zookeeper.Dial(zkAddr, 15e9)
	c.Assert(err, IsNil)
	event := <-session
	c.Assert(event.Ok(), Equals, true)
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)
	testing.ZkRemoveTree(zk, "/")
}

func (f *UnitFixture) AssertUnitCommand(c *C, name string) {
	ctx := &server.ClientContext{Id: "TestCtx", State: f.ctx.State}
	com, err := ctx.NewCommand(name)
	c.Assert(com, IsNil)
	c.Assert(err, ErrorMatches, "context TestCtx is not attached to a unit")

	ctx = &server.ClientContext{Id: "TestCtx", LocalUnitName: f.ctx.LocalUnitName}
	com, err = ctx.NewCommand(name)
	c.Assert(com, IsNil)
	c.Assert(err, ErrorMatches, "context TestCtx cannot access state")
}

type TruthErrorSuite struct{}

var _ = Suite(&TruthErrorSuite{})

var truthErrorTests = []struct {
	value interface{}
	err   error
}{
	{0, cmd.ErrSilent},
	{int8(0), cmd.ErrSilent},
	{int16(0), cmd.ErrSilent},
	{int32(0), cmd.ErrSilent},
	{int64(0), cmd.ErrSilent},
	{uint(0), cmd.ErrSilent},
	{uint8(0), cmd.ErrSilent},
	{uint16(0), cmd.ErrSilent},
	{uint32(0), cmd.ErrSilent},
	{uint64(0), cmd.ErrSilent},
	{uintptr(0), cmd.ErrSilent},
	{123, nil},
	{int8(123), nil},
	{int16(123), nil},
	{int32(123), nil},
	{int64(123), nil},
	{uint(123), nil},
	{uint8(123), nil},
	{uint16(123), nil},
	{uint32(123), nil},
	{uint64(123), nil},
	{uintptr(123), nil},
	{0.0, cmd.ErrSilent},
	{float32(0.0), cmd.ErrSilent},
	{123.45, nil},
	{float32(123.45), nil},
	{nil, cmd.ErrSilent},
	{"", cmd.ErrSilent},
	{"blah", nil},
	{true, nil},
	{false, cmd.ErrSilent},
	{[]string{}, cmd.ErrSilent},
	{[]string{""}, nil},
	{[]bool{}, cmd.ErrSilent},
	{[]bool{false}, nil},
	{map[string]string{}, cmd.ErrSilent},
	{map[string]string{"": ""}, nil},
	{map[bool]bool{}, cmd.ErrSilent},
	{map[bool]bool{false: false}, nil},
	{struct{ x bool }{false}, nil},
}

func (s *TruthErrorSuite) TestTruthError(c *C) {
	for _, t := range truthErrorTests {
		c.Assert(server.TruthError(t.value), Equals, t.err)
	}
}
