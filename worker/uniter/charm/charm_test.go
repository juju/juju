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

type StateFileSuite struct{}

var _ = Suite(&StateFileSuite{})

func (s *StateFileSuite) TestStateFile(c *C) {
	path := filepath.Join(c.MkDir(), "charm")
	f := charm.NewStateFile(path)
	st, err := f.Read()
	c.Assert(err, IsNil)
	c.Assert(st, Equals, charm.Missing)

	err = ioutil.WriteFile(path, []byte("roflcopter"), 0644)
	c.Assert(err, IsNil)
	_, err = f.Read()
	c.Assert(err, ErrorMatches, "invalid charm state at "+path)

	bad := func() { f.Write(charm.Status("claptrap")) }
	c.Assert(bad, PanicMatches, `invalid charm status "claptrap"`)

	bad = func() { f.Write(charm.Missing) }
	c.Assert(bad, PanicMatches, `invalid charm status ""`)

	err = f.Write(charm.Installed)
	c.Assert(err, IsNil)
	st, err = f.Read()
	c.Assert(err, IsNil)
	c.Assert(st, Equals, charm.Installed)
}

type BundlesDirSuite struct {
	coretesting.HTTPSuite
	testing.JujuConnSuite
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
	curl := corecharm.MustParseURL("cs:series/dummy-1")
	surl, err := url.Parse(s.URL("/some/charm.bundle"))
	c.Assert(err, IsNil)
	bunpath := coretesting.Charms.BundlePath(c.MkDir(), "dummy")
	bun, err := corecharm.ReadBundle(bunpath)
	c.Assert(err, IsNil)
	bundata, hash := readHash(c, bunpath)
	sch, err := s.State.AddCharm(bun, curl, surl, hash)
	c.Assert(err, IsNil)

	// Try to get the charm when the content doesn't match.
	coretesting.Server.Response(200, nil, []byte("roflcopter"))
	var t tomb.Tomb
	_, err = d.Read(sch, &t)
	prefix := fmt.Sprintf(`failed to download charm "cs:series/dummy-1" from %q: `, surl)
	c.Assert(err, ErrorMatches, prefix+fmt.Sprintf(`expected sha256 %q, got ".*"`, hash))
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
