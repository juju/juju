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
	"launchpad.net/tomb"
	"net/url"
	"os"
	"path/filepath"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

type CharmSuite struct {
	coretesting.HTTPSuite
	testing.JujuConnSuite
}

func (s *CharmSuite) TearDownTest(c *C) {
	s.HTTPSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *CharmSuite) AddCharm(c *C) (*state.Charm, []byte) {
	curl := corecharm.MustParseURL("cs:series/dummy-1")
	surl, err := url.Parse(s.URL("/some/charm.bundle"))
	c.Assert(err, IsNil)
	bunpath := coretesting.Charms.BundlePath(c.MkDir(), "dummy")
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

func (s *ManagerSuite) TestReadURL(c *C) {
	mgr := charm.NewManager(c.MkDir(), c.MkDir())
	_, err := mgr.ReadURL()
	c.Assert(err, Equals, charm.ErrMissing)

	setCharmURL(c, mgr, "roflcopter")
	_, err = mgr.ReadURL()
	c.Assert(err, ErrorMatches, `charm URL has invalid schema: "roflcopter"`)

	surl := "cs:series/minecraft-90210"
	setCharmURL(c, mgr, surl)
	url, err := mgr.ReadURL()
	c.Assert(err, IsNil)
	c.Assert(url, DeepEquals, corecharm.MustParseURL(surl))
}

func (s *ManagerSuite) TestStatus(c *C) {
	mgr := charm.NewManager(c.MkDir(), c.MkDir())
	_, _, err := mgr.ReadStatus()
	c.Assert(err, Equals, charm.ErrMissing)

	err = mgr.WriteStatus(charm.Installed, nil)
	c.Assert(err, IsNil)
	_, _, err = mgr.ReadStatus()
	c.Assert(err, Equals, charm.ErrMissing)

	charmURL := "cs:series/expansion-123"
	setCharmURL(c, mgr, charmURL)
	st, url, err := mgr.ReadStatus()
	c.Assert(err, IsNil)
	c.Assert(st, Equals, charm.Installed)
	c.Assert(url, DeepEquals, corecharm.MustParseURL(charmURL))

	statusURL := corecharm.MustParseURL("cs:series/contraction-987")
	for _, expect := range []charm.Status{
		charm.Installing, charm.Upgrading, charm.Conflicted,
	} {
		err = mgr.WriteStatus(expect, statusURL)
		c.Assert(err, IsNil)
		st, url, err := mgr.ReadStatus()
		c.Assert(err, IsNil)
		c.Assert(st, Equals, expect)
		c.Assert(url, DeepEquals, statusURL)
	}
}

func (s *ManagerSuite) TestUpdate(c *C) {
	// TODO: reimplement SUT using bzr; write much much nastier tests.
	mgr := charm.NewManager(c.MkDir(), c.MkDir())
	sch, bundata := s.AddCharm(c)
	err := os.Chmod(mgr.Path(), 0555)
	c.Assert(err, IsNil)
	defer os.Chmod(mgr.Path(), 0755)

	coretesting.Server.Response(200, nil, bundata)
	t := tomb.Tomb{}
	err = mgr.Update(sch, &t)
	c.Assert(err, ErrorMatches, fmt.Sprintf("failed to write charm to %s: .*", mgr.Path()))
	_, err = mgr.ReadURL()
	c.Assert(err, Equals, charm.ErrMissing)

	err = os.Chmod(mgr.Path(), 0755)
	c.Assert(err, IsNil)
	err = mgr.Update(sch, &t)
	c.Assert(err, IsNil)
	curl, err := mgr.ReadURL()
	c.Assert(err, IsNil)
	c.Assert(curl, DeepEquals, sch.URL())
}

func setCharmURL(c *C, mgr *charm.Manager, url string) {
	path := filepath.Join(mgr.Path(), ".juju-charm")
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
	var t tomb.Tomb
	_, err = d.Read(sch, &t)
	prefix := fmt.Sprintf(`failed to download charm "cs:series/dummy-1" from %q: `, sch.BundleURL())
	c.Assert(err, ErrorMatches, prefix+fmt.Sprintf(`expected sha256 %q, got ".*"`, sch.BundleSha256()))
	c.Assert(t.Err(), Equals, tomb.ErrStillAlive)

	// Try to get a charm whose bundle doesn't exist.
	coretesting.Server.Response(404, nil, nil)
	_, err = d.Read(sch, &t)
	c.Assert(err, ErrorMatches, prefix+`.* 404 Not Found`)
	c.Assert(t.Err(), Equals, tomb.ErrStillAlive)

	// Get a charm whose bundle exists and whose content matches.
	coretesting.Server.Response(200, nil, bundata)
	ch, err := d.Read(sch, &t)
	c.Assert(err, IsNil)
	assertCharm(c, ch, sch)
	c.Assert(t.Err(), Equals, tomb.ErrStillAlive)

	// Get the same charm again, without preparing a response from the server.
	ch, err = d.Read(sch, &t)
	c.Assert(err, IsNil)
	assertCharm(c, ch, sch)
	c.Assert(t.Err(), Equals, tomb.ErrStillAlive)

	// Abort a download.
	err = os.RemoveAll(bunsdir)
	c.Assert(err, IsNil)
	done := make(chan bool)
	go func() {
		ch, err := d.Read(sch, &t)
		c.Assert(ch, IsNil)
		c.Assert(err, ErrorMatches, prefix+"aborted")
		close(done)
	}()
	t.Kill(fmt.Errorf("some unrelated error"))
	<-done
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
