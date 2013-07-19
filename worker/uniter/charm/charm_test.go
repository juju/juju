// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"

	corecharm "launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker/uniter/charm"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type BundlesDirSuite struct {
	coretesting.HTTPSuite
	testing.JujuConnSuite
}

var _ = Suite(&BundlesDirSuite{})

func (s *BundlesDirSuite) SetUpSuite(c *C) {
	s.HTTPSuite.SetUpSuite(c)
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *BundlesDirSuite) TearDownSuite(c *C) {
	s.JujuConnSuite.TearDownSuite(c)
	s.HTTPSuite.TearDownSuite(c)
}

func (s *BundlesDirSuite) SetUpTest(c *C) {
	s.HTTPSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)
}

func (s *BundlesDirSuite) TearDownTest(c *C) {
	s.JujuConnSuite.TearDownTest(c)
	s.HTTPSuite.TearDownTest(c)
}

func (s *BundlesDirSuite) AddCharm(c *C) (*state.Charm, []byte) {
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

func (s *BundlesDirSuite) TestGet(c *C) {
	basedir := c.MkDir()
	bunsdir := filepath.Join(basedir, "random", "bundles")
	d := charm.NewBundlesDir(bunsdir)

	// Check it doesn't get created until it's needed.
	_, err := os.Stat(bunsdir)
	c.Assert(err, checkers.Satisfies, os.IsNotExist)

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
	case <-time.After(coretesting.LongWait):
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
