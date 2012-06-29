package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"net/url"
	stdtesting "testing"
	"time"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

// ConnSuite is a testing.StateSuite with direct access to the
// State's underlying zookeeper.Conn.
type ConnSuite struct {
	testing.StateSuite
	zkConn *zookeeper.Conn
}

func (s *ConnSuite) SetUpTest(c *C) {
	s.StateSuite.SetUpTest(c)
	s.zkConn = state.ZkConn(s.St)
}

type StateSuite struct {
	zkConn *zookeeper.Conn
	st     *state.State
	ch     charm.Charm
	curl   *charm.URL
}

var _ = Suite(&StateSuite{})

func (s *StateSuite) SetUpTest(c *C) {
	var err error
	s.st, err = state.Initialize(&state.Info{
		Addrs: []string{coretesting.ZkAddr},
	})
	c.Assert(err, IsNil)
	s.zkConn = state.ZkConn(s.st)
	s.ch = coretesting.Charms.Dir("dummy")
	url := fmt.Sprintf("local:series/%s-%d", s.ch.Meta().Name, s.ch.Revision())
	s.curl = charm.MustParseURL(url)
}

func (s *StateSuite) TearDownTest(c *C) {
	coretesting.ZkRemoveTree(s.zkConn, "/")
	s.zkConn.Close()
}

func (s *StateSuite) assertMachineCount(c *C, expect int) {
	ms, err := s.st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, expect)
}

func (s *StateSuite) TestInitialize(c *C) {
	info := &state.Info{
		Addrs: []string{coretesting.ZkAddr},
	}
	// Check that initialization of an already-initialized state succeeds.
	st, err := state.Initialize(info)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	st.Close()

	// Check that Open blocks until Initialize has succeeded.
	coretesting.ZkRemoveTree(s.zkConn, "/")

	errc := make(chan error)
	go func() {
		st, err := state.Open(info)
		errc <- err
		if st != nil {
			st.Close()
		}
	}()

	// Wait a little while to verify that it's actually blocking.
	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-errc:
		c.Fatalf("state.Open did not block (returned error %v)", err)
	default:
	}

	st, err = state.Initialize(info)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	defer st.Close()

	select {
	case err := <-errc:
		c.Assert(err, IsNil)
	case <-time.After(1e9):
		c.Fatalf("state.Open blocked forever")
	}
}

func (s *StateSuite) TestAddCharm(c *C) {
	// Check that adding charms works correctly.
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := s.st.AddCharm(s.ch, s.curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, s.curl.String())
	children, _, err := s.zkConn.Children("/charms")
	c.Assert(err, IsNil)
	c.Assert(children, DeepEquals, []string{"local_3a_series_2f_dummy-1"})
}

// addDummyCharm adds the 'dummy' charm state to st.
func (s *StateSuite) addDummyCharm(c *C) *state.Charm {
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := s.st.AddCharm(s.ch, s.curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	return dummy
}

func (s *StateSuite) TestCharmAttributes(c *C) {
	// Check that the basic (invariant) fields of the charm
	// are correctly in place.
	s.addDummyCharm(c)

	dummy, err := s.st.Charm(s.curl)
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, s.curl.String())
	c.Assert(dummy.Revision(), Equals, 1)
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	c.Assert(dummy.BundleURL(), DeepEquals, bundleURL)
	c.Assert(dummy.BundleSha256(), Equals, "dummy-sha256")
	meta := dummy.Meta()
	c.Assert(meta.Name, Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
}

func (s *StateSuite) TestNonExistentCharmPriorToInitialization(c *C) {
	// Check that getting a charm before any other charm has been added fails nicely.
	_, err := s.st.Charm(s.curl)
	c.Assert(err, ErrorMatches, `can't get charm "local:series/dummy-1": .*`)
}

func (s *StateSuite) TestGetNonExistentCharm(c *C) {
	// Check that getting a non-existent charm fails nicely.
	s.addDummyCharm(c)

	curl := charm.MustParseURL("local:anotherseries/dummy-1")
	_, err := s.st.Charm(curl)
	c.Assert(err, ErrorMatches, `can't get charm "local:anotherseries/dummy-1": .*`)
}

// addLoggingCharm adds a "logging" (subordinate) charm
// to the state.
func addLoggingCharm(c *C, st *state.State) *state.Charm {
	bundle := coretesting.Charms.Bundle(c.MkDir(), "logging")
	curl := charm.MustParseURL("cs:series/logging-99")
	bundleURL, err := url.Parse("http://subordinate.url")
	c.Assert(err, IsNil)
	ch, err := st.AddCharm(bundle, curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	return ch
}

func (s *StateSuite) TestEnvironConfig(c *C) {
	path, err := s.zkConn.Create("/environment", "type: dummy\nname: foo\n", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
	c.Assert(path, Equals, "/environment")

	env, err := s.st.EnvironConfig()
	env.Read()
	c.Assert(err, IsNil)
	c.Assert(env.Map(), DeepEquals, map[string]interface{}{"type": "dummy", "name": "foo"})
}
