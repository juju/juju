package state_test

import (
	"fmt"
	"io/ioutil"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"net/url"
	"path/filepath"
	stdtesting "testing"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

// ConnSuite provides the infrastructure for all other
// test suites (StateSuite, CharmSuite, MachineSuite, etc).
type ConnSuite struct {
	testing.MgoSuite
	testing.LoggingSuite
	charms    *mgo.Collection
	machines  *mgo.Collection
	relations *mgo.Collection
	services  *mgo.Collection
	units     *mgo.Collection
	State     *state.State
}

func (cs *ConnSuite) SetUpSuite(c *C) {
	cs.LoggingSuite.SetUpSuite(c)
	cs.MgoSuite.SetUpSuite(c)
}

func (cs *ConnSuite) TearDownSuite(c *C) {
	cs.MgoSuite.TearDownSuite(c)
	cs.LoggingSuite.TearDownSuite(c)
}

func (cs *ConnSuite) SetUpTest(c *C) {
	cs.LoggingSuite.SetUpTest(c)
	cs.MgoSuite.SetUpTest(c)
	cs.charms = cs.MgoSuite.Session.DB("juju").C("charms")
	cs.machines = cs.MgoSuite.Session.DB("juju").C("machines")
	cs.relations = cs.MgoSuite.Session.DB("juju").C("relations")
	cs.services = cs.MgoSuite.Session.DB("juju").C("services")
	cs.units = cs.MgoSuite.Session.DB("juju").C("units")
	var err error
	cs.State, err = state.Open(state.TestingStateInfo())
	c.Assert(err, IsNil)
}

func (cs *ConnSuite) TearDownTest(c *C) {
	cs.State.Close()
	cs.MgoSuite.TearDownTest(c)
	cs.LoggingSuite.TearDownTest(c)
}

func (s *ConnSuite) addCharm(c *C, ch charm.Charm) *state.Charm {
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := s.State.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
}

func (s *ConnSuite) AddTestingCharm(c *C, name string) *state.Charm {
	return s.addCharm(c, testing.Charms.Dir(name))
}

func (s *ConnSuite) AddConfigCharm(c *C, name, configYaml string, revision int) *state.Charm {
	path := testing.Charms.ClonedDirPath(c.MkDir(), name)
	config := filepath.Join(path, "config.yaml")
	err := ioutil.WriteFile(config, []byte(configYaml), 0644)
	c.Assert(err, IsNil)
	ch, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	ch.SetRevision(revision)
	return s.addCharm(c, ch)
}
