// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	jujucharm "github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/downloader"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/testcharms"
)

type BundlesDirSuite struct {
	testhelpers.IsolationSuite
}

func TestBundlesDirSuite(t *stdtesting.T) { tc.Run(t, &BundlesDirSuite{}) }

type fakeBundleInfo struct {
	charm.BundleInfo
	curl   string
	sha256 string
}

func (f fakeBundleInfo) URL() string {
	if f.curl == "" {
		return f.BundleInfo.URL()
	}
	return f.curl
}

func (f fakeBundleInfo) ArchiveSha256(ctx context.Context) (string, error) {
	if f.sha256 == "" {
		return f.BundleInfo.ArchiveSha256(ctx)
	}
	return f.sha256, nil
}

func (s *BundlesDirSuite) testCharm(c *tc.C) *charmtesting.CharmDir {
	base := testcharms.Repo.ClonedDirPath(c.MkDir(), "wordpress")
	dir, err := charmtesting.ReadCharmDir(base)
	c.Assert(err, tc.ErrorIsNil)
	return dir
}

func (s *BundlesDirSuite) TestGet(c *tc.C) {
	basedir := c.MkDir()
	bunsDir := filepath.Join(basedir, "random", "bundles")

	sch := s.testCharm(c)

	var buf bytes.Buffer
	err := sch.ArchiveTo(&buf)
	c.Assert(err, tc.ErrorIsNil)
	hash, _, err := utils.ReadSHA256(&buf)
	c.Assert(err, tc.ErrorIsNil)

	dlr := &downloader.Downloader{
		OpenBlob: func(req downloader.Request) (io.ReadCloser, error) {
			curl := jujucharm.MustParseURL(req.URL.String())
			if curl.Name != sch.Meta().Name {
				return nil, errors.NotFoundf(req.URL.String())
			}
			var buf bytes.Buffer
			err := sch.ArchiveTo(&buf)
			return io.NopCloser(&buf), err
		},
	}
	d := charm.NewBundlesDir(bunsDir, dlr, loggertesting.WrapCheckLog(c))

	checkDownloadsEmpty := func() {
		files, err := os.ReadDir(filepath.Join(bunsDir, "downloads"))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(files, tc.HasLen, 0)
	}

	// Check it doesn't get created until it's needed.
	_, err = os.Stat(bunsDir)
	c.Assert(err, tc.Satisfies, os.IsNotExist)

	// Add a charm to state that we can try to get.
	apiCharm := &fakeBundleInfo{
		curl:   "ch:quantal/wordpress-1",
		sha256: hash,
	}

	// Try to get the charm when the content doesn't match.
	_, err = d.Read(c.Context(), &fakeBundleInfo{BundleInfo: apiCharm, curl: "", sha256: "..."})
	c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "ch:quantal/wordpress-1" from API server: `)+`expected sha256 "...", got ".*"`)
	checkDownloadsEmpty()

	// Try to get a charm whose bundle doesn't exist.
	_, err = d.Read(c.Context(), &fakeBundleInfo{BundleInfo: apiCharm, curl: "ch:quantal/spam-1", sha256: ""})
	c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "ch:quantal/spam-1" from API server: `)+`.* not found`)
	checkDownloadsEmpty()

	// Get a charm whose bundle exists and whose content matches.
	ch, err := d.Read(c.Context(), apiCharm)
	c.Assert(err, tc.ErrorIsNil)
	assertCharm(c, ch, sch)
	checkDownloadsEmpty()

	// Get the same charm again, without preparing a response from the server.
	ch, err = d.Read(c.Context(), apiCharm)
	c.Assert(err, tc.ErrorIsNil)
	assertCharm(c, ch, sch)
	checkDownloadsEmpty()

	// Check the abort chan is honoured.
	err = os.RemoveAll(bunsDir)
	c.Assert(err, tc.ErrorIsNil)

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	ch, err = d.Read(ctx, apiCharm)
	c.Check(ch, tc.IsNil)
	c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(`failed to download charm "ch:quantal/wordpress-1" from API server: download aborted`))
	checkDownloadsEmpty()
}

func assertCharm(c *tc.C, bun charm.Bundle, sch *charmtesting.CharmDir) {
	actual := bun.(*jujucharm.CharmArchive)
	c.Assert(actual.Revision(), tc.Equals, sch.Revision())
	c.Assert(actual.Meta(), tc.DeepEquals, sch.Meta())
	c.Assert(actual.Config(), tc.DeepEquals, sch.Config())
}

type ClearDownloadsSuite struct {
	testhelpers.IsolationSuite
}

func TestClearDownloadsSuite(t *stdtesting.T) { tc.Run(t, &ClearDownloadsSuite{}) }
func (s *ClearDownloadsSuite) TestWorks(c *tc.C) {
	baseDir := c.MkDir()
	bunsDir := filepath.Join(baseDir, "bundles")
	downloadDir := filepath.Join(bunsDir, "downloads")
	c.Assert(os.MkdirAll(downloadDir, 0777), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(downloadDir, "stuff"), []byte("foo"), 0755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(downloadDir, "thing"), []byte("bar"), 0755), tc.ErrorIsNil)

	err := charm.ClearDownloads(bunsDir)
	c.Assert(err, tc.ErrorIsNil)
	checkMissing(c, downloadDir)
}

func (s *ClearDownloadsSuite) TestEmptyOK(c *tc.C) {
	baseDir := c.MkDir()
	bunsDir := filepath.Join(baseDir, "bundles")
	downloadDir := filepath.Join(bunsDir, "downloads")
	c.Assert(os.MkdirAll(downloadDir, 0777), tc.ErrorIsNil)

	err := charm.ClearDownloads(bunsDir)
	c.Assert(err, tc.ErrorIsNil)
	checkMissing(c, downloadDir)
}

func (s *ClearDownloadsSuite) TestMissingOK(c *tc.C) {
	baseDir := c.MkDir()
	bunsDir := filepath.Join(baseDir, "bundles")

	err := charm.ClearDownloads(bunsDir)
	c.Assert(err, tc.ErrorIsNil)
}

func checkMissing(c *tc.C, p string) {
	_, err := os.Stat(p)
	if !os.IsNotExist(err) {
		c.Fatalf("checking %s is missing: %v", p, err)
	}
}
