// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testcharms holds a corpus of charms
// for testing.
package testcharms

import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/charm/v7"
	"github.com/juju/charmrepo/v5/testing"

	jtesting "github.com/juju/juju/testing"
)

const defaultSeries = "quantal"
const localCharmRepo = "charm-repo"

// Repo provides access to the test charm repository.
var Repo = testing.NewRepo(localCharmRepo, defaultSeries)

// RepoForSeries returns a new charm repository for the specified series.
// Note: this is a bit weird, as it ignores the series if it's NOT kubernetes
// and falls back to the default series, which makes this pretty pointless.
func RepoForSeries(series string) *testing.Repo {
	// TODO(ycliuhw): workaround - currently `quantal` is not exact series
	// (for example, here makes deploy charm at charm-repo/quantal/mysql --series precise possible )!
	if series != "kubernetes" {
		series = defaultSeries
	}
	return testing.NewRepo(localCharmRepo, series)
}

// RepoWithSeries returns a new charm repository for the specified series.
func RepoWithSeries(series string) *testing.Repo {
	return testing.NewRepo(localCharmRepo, series)
}

// CheckCharmReady ensures that a desired charm archive exists and
// has some content.
func CheckCharmReady(c *gc.C, charmArchive *charm.CharmArchive) {
	fileSize := func() int64 {
		f, err := os.Open(charmArchive.Path)
		c.Assert(err, jc.ErrorIsNil)
		defer f.Close()

		fi, err := f.Stat()
		c.Assert(err, jc.ErrorIsNil)
		return fi.Size()
	}

	var oldSize, currentSize int64
	var charmReady bool
	runs := 1
	timeout := time.After(jtesting.LongWait)
	for !charmReady {
		select {
		case <-time.After(jtesting.ShortWait):
			currentSize = fileSize()
			// Since we do not know when the charm is ready, for as long as the size changes
			// we'll assume that we'd need to wait.
			charmReady = oldSize != 0 && currentSize == oldSize
			c.Logf("%d: new file size %v (old size %v)", runs, currentSize, oldSize)
			oldSize = currentSize
			runs++
		case <-timeout:
			c.Fatalf("timed out waiting for charm @%v to be ready", charmArchive.Path)
		}
	}
}

// InjectFilesToCharmArchive overwrites the contents of pathToArchive with a
// new archive containing the original files plus the ones provided in the
// fileContents map (key: file name, value: file contents).
func InjectFilesToCharmArchive(pathToArchive string, fileContents map[string]string) error {
	zr, err := zip.OpenReader(pathToArchive)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	defer func() { _ = zw.Close() }()

	// Copy existing files
	for _, f := range zr.File {
		w, err := zw.CreateHeader(&f.FileHeader)
		if err != nil {
			return err
		}

		r, err := f.Open()
		if err != nil {
			return err
		}

		_, err = io.Copy(w, r)
		_ = r.Close()
		if err != nil {
			return err
		}
	}

	// Add new files
	for name, contents := range fileContents {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}

		if _, err = w.Write([]byte(contents)); err != nil {
			return err
		}
	}

	// Overwrite original archive with the patched version
	_, _ = zr.Close(), zw.Close()
	return ioutil.WriteFile(pathToArchive, buf.Bytes(), 0644)
}
