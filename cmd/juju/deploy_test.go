package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

var zkConfig = `
environments:
    palermo:
        type: dummy
        zookeeper: true
        authorized-keys: i-am-a-key
`

type DeploySuite struct {
	envSuite
	seriesPath string
	repoPath   string
	conn       *juju.Conn
	st         *state.State
}

var _ = Suite(&DeploySuite{})

func (s *DeploySuite) SetUpTest(c *C) {
	s.envSuite.SetUpTest(c, zkConfig)
	repoPath := c.MkDir()
	s.repoPath = os.Getenv("JUJU_REPOSITORY")
	os.Setenv("JUJU_REPOSITORY", repoPath)
	s.seriesPath = filepath.Join(repoPath, "precise")
	err := os.Mkdir(s.seriesPath, 0777)
	c.Assert(err, IsNil)
	s.conn, err = juju.NewConn("")
	c.Assert(err, IsNil)
	err = s.conn.Environ.Bootstrap(false)
	c.Assert(err, IsNil)
	s.st, err = s.conn.State()
	c.Assert(err, IsNil)
}

func (s *DeploySuite) TearDownTest(c *C) {
	s.envSuite.TearDownTest(c)
	os.Setenv("JUJU_REPOSITORY", s.repoPath)
	s.conn.Close()
}

func runDeploy(c *C, args ...string) error {
	com := &DeployCommand{}
	err := com.Init(newFlagSet(), args)
	c.Assert(err, IsNil)
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *DeploySuite) assertService(c *C, name string, expectCurl *charm.URL, unitCount, relCount int) []*state.Relation {
	srv, err := s.st.Service(name)
	c.Assert(err, IsNil)
	curl, err := srv.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(curl, DeepEquals, expectCurl)
	s.assertCharmUploaded(c, curl)
	units, err := srv.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, unitCount)
	s.assertUnitMachines(c, units)
	rels, err := srv.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, relCount)
	return rels
}

func (s *DeploySuite) assertCharmUploaded(c *C, curl *charm.URL) {
	ch, err := s.st.Charm(curl)
	c.Assert(err, IsNil)
	url := ch.BundleURL()
	resp, err := http.Get(url.String())
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	h := sha256.New()
	h.Write(body)
	digest := hex.EncodeToString(h.Sum(nil))
	c.Assert(ch.BundleSha256(), Equals, digest)
}

func (s *DeploySuite) assertUnitMachines(c *C, units []*state.Unit) {
	expectUnitNames := []string{}
	for _, u := range units {
		expectUnitNames = append(expectUnitNames, u.Name())
	}
	sort.Strings(expectUnitNames)

	machines, err := s.st.AllMachines()
	c.Assert(err, IsNil)
	// NOTE: this will fail when state.Initialize starts doing
	// the right thing and poking machine 0 into ZK state.
	c.Assert(machines, HasLen, len(units))
	unitNames := []string{}
	for _, m := range machines {
		mUnits, err := m.Units()
		c.Assert(err, IsNil)
		c.Assert(mUnits, HasLen, 1)
		unitNames = append(unitNames, mUnits[0].Name())
	}
	sort.Strings(unitNames)
	c.Assert(unitNames, DeepEquals, expectUnitNames)
}

func (s *DeploySuite) TestCharmDir(c *C) {
	testing.Charms.ClonedDirPath(s.seriesPath, "dummy")
	err := runDeploy(c, "local:dummy")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.assertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeCharmDir(c *C) {
	dirPath := testing.Charms.ClonedDirPath(s.seriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-u")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-2")
	s.assertService(c, "dummy", curl, 1, 0)
	// Check the charm really was upgraded.
	ch, err := charm.ReadDir(dirPath)
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, 2)
}

func (s *DeploySuite) TestCharmBundle(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.assertService(c, "some-service-name", curl, 1, 0)
}

func (s *DeploySuite) TestCannotUpgradeCharmBundle(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-u")
	c.Assert(err, ErrorMatches, `cannot increment version of charm "local:precise/dummy-1": not a directory`)
	// Verify state not touched...
	curl := charm.MustParseURL("local:precise/dummy-1")
	_, err = s.st.Charm(curl)
	c.Assert(err, ErrorMatches, `cannot get charm "local:precise/dummy-1": charm not found`)
	_, err = s.st.Service("dummy")
	c.Assert(err, ErrorMatches, `cannot get service "dummy": service with name "dummy" not found`)
}

func (s *DeploySuite) TestAddsPeerRelations(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "riak")
	err := runDeploy(c, "local:riak")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/riak-7")
	rels := s.assertService(c, "riak", curl, 1, 1)
	rel := rels[0]
	ep, err := rel.Endpoint("riak")
	c.Assert(err, IsNil)
	c.Assert(ep.RelationName, Equals, "ring")
	c.Assert(ep.RelationRole, Equals, state.RolePeer)
	c.Assert(ep.RelationScope, Equals, charm.ScopeGlobal)
}

func (s *DeploySuite) TestNumUnits(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-n", "13")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.assertService(c, "dummy", curl, 13, 0)
}

func (s *DeploySuite) TestSubordinateCharm(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "logging")
	err := runDeploy(c, "local:logging")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/logging-1")
	s.assertService(c, "logging", curl, 0, 0)
}
