package juju_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
)

var _ = Suite(&DeploySuite{})

type DeploySuite struct {
	testing.ZkSuite
	conn  *juju.Conn
	state *state.State
}

func (s *DeploySuite) SetUpTest(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	}
	conn, err := juju.NewConnFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = conn.Bootstrap(false)
	c.Assert(err, IsNil)
	st, err := conn.State()
	c.Assert(err, IsNil)
	s.conn = conn
	s.state = st
}

func (s *DeploySuite) TearDownTest(c *C) {
	if s.conn == nil {
		return
	}
	err := s.conn.Destroy()
	c.Check(err, IsNil)
	s.conn.Close()
	s.conn = nil
}

func (s *DeploySuite) TestPutCharmBasic(c *C) {
	repoPath := c.MkDir()
	curl := testing.Charms.ClonedURL(repoPath, "riak")
	curl.Revision = -1 // make sure we trigger the repo.Latest logic.
	sch, err := s.conn.PutCharm(curl, repoPath, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	sch, err = s.state.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}

func (s *DeploySuite) TestPutBundledCharm(c *C) {
	// Bundle the riak charm into a charm repo directory.
	repoPath := c.MkDir()
	dir := filepath.Join(repoPath, "series")
	err := os.Mkdir(dir, 0777)
	c.Assert(err, IsNil)
	w, err := os.Create(filepath.Join(dir, "riak.charm"))
	c.Assert(err, IsNil)
	defer w.Close()
	charmDir := testing.Charms.Dir("riak")
	err = charmDir.BundleTo(w)
	c.Assert(err, IsNil)

	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		c.Logf("%s %v", path, info.IsDir())
		return nil
	})
	// Invent a URL that points to the bundled charm, and
	// test putting that.
	curl := &charm.URL{
		Schema:   "local",
		Series:   "series",
		Name:     "riak",
		Revision: -1,
	}
	sch, err := s.conn.PutCharm(curl, repoPath, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	// Check that we can get the charm from the state.
	sch, err = s.state.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}

func (s *DeploySuite) TestPutCharmBumpRevision(c *C) {
	repo := &charm.LocalRepository{c.MkDir()}
	curl := testing.Charms.ClonedURL(repo.Path, "riak")

	// Put charm for the first time.
	sch, err := s.conn.PutCharm(curl, repo.Path, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	sch, err = s.state.Charm(sch.URL())
	c.Assert(err, IsNil)
	sha256 := sch.BundleSha256()

	// Change the charm on disk.
	ch, err := repo.Get(curl)
	c.Assert(err, IsNil)
	chd := ch.(*charm.Dir)
	err = ioutil.WriteFile(filepath.Join(chd.Path, "extra"), []byte("arble"), 0666)
	c.Assert(err, IsNil)

	// Put charm again and check that it has not changed in the state.
	sch, err = s.conn.PutCharm(curl, repo.Path, false)
	c.Assert(err, IsNil)

	sch, err = s.state.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Equals, sha256)

	// Put charm again, with bumpRevision this time, and check that
	// it has changed.

	sch, err = s.conn.PutCharm(curl, repo.Path, true)
	c.Assert(err, IsNil)

	sch, err = s.state.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Not(Equals), sha256)
}

//func (ConnSuite) TestNewService(c *C) {
