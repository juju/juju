// Copyright 2012-2014 Canonical Ltd.
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
	"time"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/charm"
)

type BundlesDirSuite struct {
	gitjujutesting.HTTPSuite
	testing.JujuConnSuite

	st     *api.State
	uniter *uniter.State
}

var _ = gc.Suite(&BundlesDirSuite{})

func (s *BundlesDirSuite) SetUpSuite(c *gc.C) {
	s.HTTPSuite.SetUpSuite(c)
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *BundlesDirSuite) TearDownSuite(c *gc.C) {
	s.JujuConnSuite.TearDownSuite(c)
	s.HTTPSuite.TearDownSuite(c)
}

func (s *BundlesDirSuite) SetUpTest(c *gc.C) {
	s.HTTPSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)

	// Add a charm, service and unit to login to the API with.
	charm := s.AddTestingCharm(c, "wordpress")
	service := s.AddTestingService(c, "wordpress", charm)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)

	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	c.Assert(s.st, gc.NotNil)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, gc.IsNil)
	c.Assert(s.uniter, gc.NotNil)
}

func (s *BundlesDirSuite) TearDownTest(c *gc.C) {
	err := s.st.Close()
	c.Assert(err, gc.IsNil)
	s.JujuConnSuite.TearDownTest(c)
	s.HTTPSuite.TearDownTest(c)
}

func (s *BundlesDirSuite) AddCharm(c *gc.C) (charm.BundleInfo, *state.Charm, []byte) {
	curl := corecharm.MustParseURL("cs:quantal/dummy-1")
	storagePath := "dummy-1"
	bunpath := testcharms.Repo.CharmArchivePath(c.MkDir(), "dummy")
	bun, err := corecharm.ReadCharmArchive(bunpath)
	c.Assert(err, gc.IsNil)
	bundata, hash := readHash(c, bunpath)
	sch, err := s.State.AddCharm(bun, curl, storagePath, hash)
	c.Assert(err, gc.IsNil)
	apiCharm, err := s.uniter.Charm(sch.URL())
	c.Assert(err, gc.IsNil)

	surl, err := url.Parse(s.URL("/some/charm.bundle"))
	c.Assert(err, gc.IsNil)
	mock := &mockArchiveURLCharm{apiCharm, surl}
	return mock, sch, bundata
}

type mockArchiveURLCharm struct {
	charm.BundleInfo
	archiveURL *url.URL
}

func (i *mockArchiveURLCharm) ArchiveURL() *url.URL {
	return i.archiveURL
}

func (s *BundlesDirSuite) TestGet(c *gc.C) {
	basedir := c.MkDir()
	bunsdir := filepath.Join(basedir, "random", "bundles")
	d := charm.NewBundlesDir(bunsdir)

	// Check it doesn't get created until it's needed.
	_, err := os.Stat(bunsdir)
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// Add a charm to state that we can try to get.
	apiCharm, sch, bundata := s.AddCharm(c)

	// Try to get the charm when the content doesn't match.
	gitjujutesting.Server.Response(200, nil, []byte("roflcopter"))
	_, err = d.Read(apiCharm, nil)
	archiveURL := apiCharm.ArchiveURL()
	prefix := fmt.Sprintf(`failed to download charm "cs:quantal/dummy-1" from %q: `, archiveURL)
	c.Assert(err, gc.ErrorMatches, prefix+fmt.Sprintf(`expected sha256 %q, got ".*"`, sch.BundleSha256()))

	// Try to get a charm whose bundle doesn't exist.
	gitjujutesting.Server.Response(404, nil, nil)
	_, err = d.Read(apiCharm, nil)
	c.Assert(err, gc.ErrorMatches, prefix+`.* 404 Not Found`)

	// Get a charm whose bundle exists and whose content matches.
	gitjujutesting.Server.Response(200, nil, bundata)
	ch, err := d.Read(apiCharm, nil)
	c.Assert(err, gc.IsNil)
	assertCharm(c, ch, sch)

	// Get the same charm again, without preparing a response from the server.
	ch, err = d.Read(apiCharm, nil)
	c.Assert(err, gc.IsNil)
	assertCharm(c, ch, sch)

	// Abort a download.
	err = os.RemoveAll(bunsdir)
	c.Assert(err, gc.IsNil)
	abort := make(chan struct{})
	done := make(chan bool)
	go func() {
		ch, err := d.Read(apiCharm, abort)
		c.Assert(ch, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, prefix+"aborted")
		close(done)
	}()
	close(abort)
	gitjujutesting.Server.Response(500, nil, nil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for abort")
	}
}

func readHash(c *gc.C, path string) ([]byte, string) {
	data, err := ioutil.ReadFile(path)
	c.Assert(err, gc.IsNil)
	hash := sha256.New()
	hash.Write(data)
	return data, hex.EncodeToString(hash.Sum(nil))
}

func assertCharm(c *gc.C, bun charm.Bundle, sch *state.Charm) {
	actual := bun.(*corecharm.CharmArchive)
	c.Assert(actual.Revision(), gc.Equals, sch.Revision())
	c.Assert(actual.Meta(), gc.DeepEquals, sch.Meta())
	c.Assert(actual.Config(), gc.DeepEquals, sch.Config())
}
