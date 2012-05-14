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
	ch := testing.Charms.Bundle(c.MkDir(), "dummy")
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
