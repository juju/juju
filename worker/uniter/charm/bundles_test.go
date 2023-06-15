// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/juju/loggo"

	corecharm "github.com/juju/charm/v11"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/worker/uniter/charm"
)

type BundlesDirSuite struct {
	testing.JujuConnSuite

	st     api.Connection
	uniter *uniter.State
}

var _ = gc.Suite(&BundlesDirSuite{})

func (s *BundlesDirSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *BundlesDirSuite) TearDownSuite(c *gc.C) {
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *BundlesDirSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Add a charm, application and unit to login to the API with.
	charm := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", charm)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	c.Assert(s.st, gc.NotNil)
	s.uniter, err = uniter.NewFromConnection(s.st)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)
}

func (s *BundlesDirSuite) TearDownTest(c *gc.C) {
	err := s.st.Close()
	c.Assert(err, jc.ErrorIsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *BundlesDirSuite) AddCharm(c *gc.C) (charm.BundleInfo, *state.Charm) {
	curl := corecharm.MustParseURL("ch:quantal/dummy-1")
	bun := testcharms.Repo.CharmDir("dummy")
	sch, err := testing.AddCharm(s.State, curl, bun, false)
	c.Assert(err, jc.ErrorIsNil)

	apiCharm, err := s.uniter.Charm(sch.URL())
	c.Assert(err, jc.ErrorIsNil)

	return apiCharm, sch
}

type fakeBundleInfo struct {
	charm.BundleInfo
	curl   string
	sha256 string
}

func (f fakeBundleInfo) String() string {
	if f.curl == "" {
		return f.BundleInfo.String()
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
	bunsDir := filepath.Join(basedir, "random", "bundles")
	downloader := charms.NewCharmDownloader(s.st)
	d := charm.NewBundlesDir(bunsDir, downloader, loggo.GetLogger(""))

	checkDownloadsEmpty := func() {
		files, err := os.ReadDir(filepath.Join(bunsDir, "downloads"))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(files, gc.HasLen, 0)
	}

	// Check it doesn't get created until it's needed.
	_, err := os.Stat(bunsDir)
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// Add a charm to state that we can try to get.
	apiCharm, sch := s.AddCharm(c)

	// Try to get the charm when the content doesn't match.
	_, err = d.Read(&fakeBundleInfo{apiCharm, "", "..."}, nil)
	c.Check(err, gc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "ch:quantal/dummy-1" from API server: `)+`expected sha256 "...", got ".*"`)
	checkDownloadsEmpty()

	// Try to get a charm whose bundle doesn't exist.
	_, err = d.Read(&fakeBundleInfo{apiCharm, "ch:quantal/spam-1", ""}, nil)
	c.Check(err, gc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "ch:quantal/spam-1" from API server: `)+`.* not found`)
	checkDownloadsEmpty()

	// Get a charm whose bundle exists and whose content matches.
	ch, err := d.Read(apiCharm, nil)
	c.Assert(err, jc.ErrorIsNil)
	assertCharm(c, ch, sch)
	checkDownloadsEmpty()

	// Get the same charm again, without preparing a response from the server.
	ch, err = d.Read(apiCharm, nil)
	c.Assert(err, jc.ErrorIsNil)
	assertCharm(c, ch, sch)
	checkDownloadsEmpty()

	// Check the abort chan is honoured.
	err = os.RemoveAll(bunsDir)
	c.Assert(err, jc.ErrorIsNil)
	abort := make(chan struct{})
	close(abort)

	ch, err = d.Read(apiCharm, abort)
	c.Check(ch, gc.IsNil)
	c.Check(err, gc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "ch:quantal/dummy-1" from API server: download aborted`))
	checkDownloadsEmpty()
}

func assertCharm(c *gc.C, bun charm.Bundle, sch *state.Charm) {
	actual := bun.(*corecharm.CharmArchive)
	c.Assert(actual.Revision(), gc.Equals, sch.Revision())
	c.Assert(actual.Meta(), gc.DeepEquals, sch.Meta())
	c.Assert(actual.Config(), gc.DeepEquals, sch.Config())
}

type ClearDownloadsSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&ClearDownloadsSuite{})

func (s *ClearDownloadsSuite) TestWorks(c *gc.C) {
	baseDir := c.MkDir()
	bunsDir := filepath.Join(baseDir, "bundles")
	downloadDir := filepath.Join(bunsDir, "downloads")
	c.Assert(os.MkdirAll(downloadDir, 0777), jc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(downloadDir, "stuff"), []byte("foo"), 0755), jc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(downloadDir, "thing"), []byte("bar"), 0755), jc.ErrorIsNil)

	err := charm.ClearDownloads(bunsDir)
	c.Assert(err, jc.ErrorIsNil)
	checkMissing(c, downloadDir)
}

func (s *ClearDownloadsSuite) TestEmptyOK(c *gc.C) {
	baseDir := c.MkDir()
	bunsDir := filepath.Join(baseDir, "bundles")
	downloadDir := filepath.Join(bunsDir, "downloads")
	c.Assert(os.MkdirAll(downloadDir, 0777), jc.ErrorIsNil)

	err := charm.ClearDownloads(bunsDir)
	c.Assert(err, jc.ErrorIsNil)
	checkMissing(c, downloadDir)
}

func (s *ClearDownloadsSuite) TestMissingOK(c *gc.C) {
	baseDir := c.MkDir()
	bunsDir := filepath.Join(baseDir, "bundles")

	err := charm.ClearDownloads(bunsDir)
	c.Assert(err, jc.ErrorIsNil)
}

func checkMissing(c *gc.C, p string) {
	_, err := os.Stat(p)
	if !os.IsNotExist(err) {
		c.Fatalf("checking %s is missing: %v", p, err)
	}
}
