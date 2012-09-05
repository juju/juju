package juju_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"path/filepath"
)

var _ = Suite(&DeploySuite{})

type DeploySuite struct {
	testing.ZkSuite
	conn *juju.Conn
	repo testing.Repo
}

func (s *DeploySuite) SetUpTest(c *C) {
	s.ZkSuite.SetUpTest(c)
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	}
	environ, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environ.Bootstrap(false)
	c.Assert(err, IsNil)
	s.conn, err = juju.NewConn(environ)
	c.Assert(err, IsNil)
	s.repo.Path = c.MkDir()
}

func (s *DeploySuite) TearDownTest(c *C) {
	if s.conn == nil {
		return
	}
	err := s.conn.Environ.Destroy(nil)
	c.Check(err, IsNil)
	s.conn.Close()
	s.conn = nil
	s.repo.Path = ""
	s.ZkSuite.TearDownTest(c)
}

func (s *DeploySuite) charmURL() *charm.URL {
	s.repo.Dir("riak")
	return s.repo.URL("series", "riak")
}

func (s *DeploySuite) TestPutCharmBasic(c *C) {
	s.repo.Dir("riak")
	curl := s.repo.URL("series", "riak")
	curl.Revision = -1 // make sure we trigger the repo.Latest logic.
	sch, err := s.conn.PutCharm(curl, s.repo.Path, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}

func (s *DeploySuite) TestPutBundledCharm(c *C) {
	s.repo.Bundle("riak")
	curl := s.repo.URL("series", "riak")

	sch, err := s.conn.PutCharm(curl, s.repo.Path, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	// Check that we can get the charm from the state.
	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}

func (s *DeploySuite) TestPutCharmUpload(c *C) {
	curl := s.charmURL()

	// Put charm for the first time.
	sch, err := s.conn.PutCharm(curl, s.repo.Path, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	sha256 := sch.BundleSha256()
	rev := sch.Revision()

	// Change the charm on disk.
	repo := charm.LocalRepository{Path: s.repo.Path}
	ch, err := repo.Get(curl)
	c.Assert(err, IsNil)
	chd := ch.(*charm.Dir)
	err = ioutil.WriteFile(filepath.Join(chd.Path, "extra"), []byte("arble"), 0666)
	c.Assert(err, IsNil)

	// Put charm again and check that it has not changed in the state.
	sch, err = s.conn.PutCharm(curl, s.repo.Path, false)
	c.Assert(err, IsNil)

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Equals, sha256)
	c.Assert(sch.Revision(), Equals, rev)

	// Put charm again, with bumpRevision this time, and check that
	// it has changed.
	sch, err = s.conn.PutCharm(curl, s.repo.Path, true)
	c.Assert(err, IsNil)

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Not(Equals), sha256)
	c.Assert(sch.Revision(), Equals, rev+1)
}

func (s *DeploySuite) TestAddService(c *C) {
	curl := s.charmURL()
	sch, err := s.conn.PutCharm(curl, s.repo.Path, false)
	c.Assert(err, IsNil)

	svc, err := s.conn.AddService("testriak", sch)
	c.Assert(err, IsNil)

	// Check that the peer relation has been made.
	relations, err := svc.Relations()
	c.Assert(relations, HasLen, 1)
	ep, err := relations[0].Endpoint("testriak")
	c.Assert(err, IsNil)
	c.Assert(ep, Equals, state.RelationEndpoint{
		ServiceName:   "testriak",
		Interface:     "riak",
		RelationName:  "ring",
		RelationRole:  state.RolePeer,
		RelationScope: charm.ScopeGlobal,
	})
}

func (s *DeploySuite) TestAddServiceDefaultName(c *C) {
	curl := s.charmURL()
	sch, err := s.conn.PutCharm(curl, s.repo.Path, false)
	c.Assert(err, IsNil)

	svc, err := s.conn.AddService("", sch)
	c.Assert(err, IsNil)
	c.Assert(svc.Name(), Equals, "riak")
}

func (s *DeploySuite) TestAddUnits(c *C) {
	curl := s.charmURL()
	sch, err := s.conn.PutCharm(curl, s.repo.Path, false)
	c.Assert(err, IsNil)
	svc, err := s.conn.AddService("testriak", sch)
	c.Assert(err, IsNil)
	units, err := s.conn.AddUnits(svc, 2)
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 2)

	id0, err := units[0].AssignedMachineId()
	c.Assert(err, IsNil)
	id1, err := units[1].AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(id0, Not(Equals), id1)
}
