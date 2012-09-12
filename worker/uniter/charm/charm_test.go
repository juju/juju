package charm_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	corecharm "launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/charm"
	"net/url"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

type CharmSuite struct {
	coretesting.HTTPSuite
	testing.JujuConnSuite
}

func (s *CharmSuite) SetUpSuite(c *C) {
	s.HTTPSuite.SetUpSuite(c)
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *CharmSuite) TearDownSuite(c *C) {
	s.JujuConnSuite.TearDownSuite(c)
	s.HTTPSuite.TearDownSuite(c)
}

func (s *CharmSuite) SetUpTest(c *C) {
	s.HTTPSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)
}

func (s *CharmSuite) TearDownTest(c *C) {
	s.JujuConnSuite.TearDownTest(c)
	s.HTTPSuite.TearDownTest(c)
}

func (s *CharmSuite) AddCharm(c *C) (*state.Charm, []byte) {
	curl := corecharm.MustParseURL("cs:series/dummy-1")
	surl, err := url.Parse(s.URL("/some/charm.bundle"))
	c.Assert(err, IsNil)
	bunpath := coretesting.Charms.BundlePath(c.MkDir(), "dummy", "series")
	bun, err := corecharm.ReadBundle(bunpath)
	c.Assert(err, IsNil)
	bundata, hash := readHash(c, bunpath)
	sch, err := s.State.AddCharm(bun, curl, surl, hash)
	c.Assert(err, IsNil)
	return sch, bundata
}

type ManagerSuite struct {
	CharmSuite
}

var _ = Suite(&ManagerSuite{})

func (s *ManagerSuite) TestStatus(c *C) {
	mgr := charm.NewManager(c.MkDir(), c.MkDir())
	_, err := mgr.ReadState()
	c.Assert(err, Equals, charm.ErrMissing)

	scharmURL := "cs:series/expansion-123"
	charmURL := corecharm.MustParseURL(scharmURL)
	err = mgr.WriteState(charm.Deployed, charmURL)
	c.Assert(err, IsNil)
	_, err = mgr.ReadState()
	c.Assert(err, Equals, charm.ErrMissing)

	setCharmURL(c, mgr, "roflcopter")
	_, err = mgr.ReadState()
	c.Assert(err, ErrorMatches, `charm URL has invalid schema: "roflcopter"`)

	statusURL := corecharm.MustParseURL("cs:series/contraction-987")
	for _, expect := range []charm.Status{
		charm.Installing, charm.Upgrading, charm.Conflicted,
	} {
		err = mgr.WriteState(expect, statusURL)
		c.Assert(err, IsNil)
		st, err := mgr.ReadState()
		c.Assert(err, IsNil)
		c.Assert(st.Status, Equals, expect)
		c.Assert(st.URL, DeepEquals, statusURL)
	}

	setCharmURL(c, mgr, scharmURL)
	err = mgr.WriteState(charm.Deployed, statusURL)
	c.Assert(err, IsNil)
	st, err := mgr.ReadState()
	c.Assert(err, IsNil)
	c.Assert(st.Status, Equals, charm.Deployed)
	c.Assert(st.URL, DeepEquals, charmURL)
}

func (s *ManagerSuite) TestUpdate(c *C) {
	// TODO: reimplement SUT using bzr; write much much nastier tests.
	mgr := charm.NewManager(c.MkDir(), c.MkDir())
	sch, bundata := s.AddCharm(c)
	err := os.Chmod(mgr.CharmDir(), 0555)
	c.Assert(err, IsNil)
	defer os.Chmod(mgr.CharmDir(), 0755)

	coretesting.Server.Response(200, nil, bundata)
	err = mgr.Update(sch, nil)
	expect := fmt.Sprintf("failed to write charm to %s: .*", mgr.CharmDir())
	c.Assert(err, ErrorMatches, expect)
	_, err = mgr.ReadState()
	c.Assert(err, Equals, charm.ErrMissing)

	err = os.Chmod(mgr.CharmDir(), 0755)
	c.Assert(err, IsNil)
	err = mgr.Update(sch, nil)
	c.Assert(err, IsNil)
	err = mgr.WriteState(charm.Deployed, corecharm.MustParseURL("cs:series/not-canonical-1"))
	st, err := mgr.ReadState()
	c.Assert(err, IsNil)
	c.Assert(st.Status, Equals, charm.Deployed)
	c.Assert(st.URL, DeepEquals, sch.URL())
}

func setCharmURL(c *C, mgr *charm.Manager, url string) {
	path := filepath.Join(mgr.CharmDir(), ".juju-charm")
	err := ioutil.WriteFile(path, []byte(url), 0644)
	c.Assert(err, IsNil)
}

type BundlesDirSuite struct {
	CharmSuite
}

var _ = Suite(&BundlesDirSuite{})

func (s *BundlesDirSuite) TestGet(c *C) {
	basedir := c.MkDir()
	bunsdir := filepath.Join(basedir, "random", "bundles")
	d := charm.NewBundlesDir(bunsdir)

	// Check it doesn't get created until it's needed.
	_, err := os.Stat(bunsdir)
	c.Assert(os.IsNotExist(err), Equals, true)

	// Add a charm to state that we can try to get.
	sch, bundata := s.AddCharm(c)

	// Try to get the charm when the content doesn't match.
	coretesting.Server.Response(200, nil, []byte("roflcopter"))
	_, err = d.Read(sch, nil)
	prefix := fmt.Sprintf(`failed to download charm "cs:series/dummy-1" from %q: `, sch.BundleURL())
	c.Assert(err, ErrorMatches, prefix+fmt.Sprintf(`expected sha256 %q, got ".*"`, sch.BundleSha256()))

	// Try to get a charm whose bundle doesn't exist.
	coretesting.Server.Response(404, nil, nil)
	_, err = d.Read(sch, nil)
	c.Assert(err, ErrorMatches, prefix+`.* 404 Not Found`)

	// Get a charm whose bundle exists and whose content matches.
	coretesting.Server.Response(200, nil, bundata)
	ch, err := d.Read(sch, nil)
	c.Assert(err, IsNil)
	assertCharm(c, ch, sch)

	// Get the same charm again, without preparing a response from the server.
	ch, err = d.Read(sch, nil)
	c.Assert(err, IsNil)
	assertCharm(c, ch, sch)

	// Abort a download.
	err = os.RemoveAll(bunsdir)
	c.Assert(err, IsNil)
	abort := make(chan struct{})
	done := make(chan bool)
	go func() {
		ch, err := d.Read(sch, abort)
		c.Assert(ch, IsNil)
		c.Assert(err, ErrorMatches, prefix+"aborted")
		close(done)
	}()
	close(abort)
	coretesting.Server.Response(500, nil, nil)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("timed out waiting for abort")
	}
}

func readHash(c *C, path string) ([]byte, string) {
	data, err := ioutil.ReadFile(path)
	c.Assert(err, IsNil)
	hash := sha256.New()
	hash.Write(data)
	return data, hex.EncodeToString(hash.Sum(nil))
}

func assertCharm(c *C, bun *corecharm.Bundle, sch *state.Charm) {
	c.Assert(bun.Revision(), Equals, sch.Revision())
	c.Assert(bun.Meta(), DeepEquals, sch.Meta())
	c.Assert(bun.Config(), DeepEquals, sch.Config())
}
