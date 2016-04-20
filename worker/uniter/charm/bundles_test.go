// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"os"
	"path/filepath"
	"regexp"
	"time"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v6-unstable"

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

	st     api.Connection
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
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	c.Assert(s.st, gc.NotNil)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)
}

func (s *BundlesDirSuite) TearDownTest(c *gc.C) {
	err := s.st.Close()
	c.Assert(err, jc.ErrorIsNil)
	s.JujuConnSuite.TearDownTest(c)
	s.HTTPSuite.TearDownTest(c)
}

func (s *BundlesDirSuite) AddCharm(c *gc.C) (charm.BundleInfo, *state.Charm) {
	curl := corecharm.MustParseURL("cs:quantal/dummy-1")
	bun := testcharms.Repo.CharmDir("dummy")
	sch, err := testing.AddCharm(s.State, curl, bun)
	c.Assert(err, jc.ErrorIsNil)

	apiCharm, err := s.uniter.Charm(sch.URL())
	c.Assert(err, jc.ErrorIsNil)

	return apiCharm, sch
}

type fakeBundleInfo struct {
	charm.BundleInfo
	curl   *corecharm.URL
	sha256 string
}

func (f fakeBundleInfo) URL() *corecharm.URL {
	if f.curl == nil {
		return f.BundleInfo.URL()
	}
	return f.curl
}

func (f fakeBundleInfo) ArchiveSha256() (string, error) {
	if f.sha256 == "" {
		return f.BundleInfo.ArchiveSha256()
	}
	return f.sha256, nil
}

func (s *BundlesDirSuite) TestGet(c *gc.C) {
	basedir := c.MkDir()
	bunsdir := filepath.Join(basedir, "random", "bundles")
	downloader := api.NewCharmDownloader(s.st.Client())
	d := charm.NewBundlesDir(bunsdir, downloader)

	// Check it doesn't get created until it's needed.
	_, err := os.Stat(bunsdir)
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// Add a charm to state that we can try to get.
	apiCharm, sch := s.AddCharm(c)

	// Try to get the charm when the content doesn't match.
	_, err = d.Read(&fakeBundleInfo{apiCharm, nil, "..."}, nil)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "cs:quantal/dummy-1" from API server: `)+`expected sha256 "...", got ".*"`)

	// Try to get a charm whose bundle doesn't exist.
	otherURL := corecharm.MustParseURL("cs:quantal/spam-1")
	_, err = d.Read(&fakeBundleInfo{apiCharm, otherURL, ""}, nil)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "cs:quantal/spam-1" from API server: `)+`.* not found`)

	// Get a charm whose bundle exists and whose content matches.
	ch, err := d.Read(apiCharm, nil)
	c.Assert(err, jc.ErrorIsNil)
	assertCharm(c, ch, sch)

	// Get the same charm again, without preparing a response from the server.
	ch, err = d.Read(apiCharm, nil)
	c.Assert(err, jc.ErrorIsNil)
	assertCharm(c, ch, sch)

	// Abort a download.
	err = os.RemoveAll(bunsdir)
	c.Assert(err, jc.ErrorIsNil)
	abort := make(chan struct{})
	done := make(chan bool)
	go func() {
		ch, err := d.Read(apiCharm, abort)
		c.Assert(ch, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "cs:quantal/dummy-1" from API server: aborted`))
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

func assertCharm(c *gc.C, bun charm.Bundle, sch *state.Charm) {
	actual := bun.(*corecharm.CharmArchive)
	c.Assert(actual.Revision(), gc.Equals, sch.Revision())
	c.Assert(actual.Meta(), gc.DeepEquals, sch.Meta())
	c.Assert(actual.Config(), gc.DeepEquals, sch.Config())
}
