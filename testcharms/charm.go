// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testcharms

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
	jtesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/testcharms/repo"
)

const (
	defaultSeries  = "quantal"
	localCharmRepo = "charm-repo"
	localCharmHub  = "charm-hub"
)

// Repo provides access to the test charm repository.
var Repo = repo.NewRepo(localCharmRepo, defaultSeries)

// Hub provides access to the test charmhub repository.
var Hub = repo.NewRepo(localCharmHub, defaultSeries)

// RepoForSeries returns a new charm repository for the specified series.
// Note: this is a bit weird, as it ignores the series if it's NOT kubernetes
// and falls back to the default series, which makes this pretty pointless.
func RepoForSeries(series string) *repo.CharmRepo {
	return repo.NewRepo(localCharmRepo, series)
}

// RepoWithSeries returns a new charm repository for the specified series.
func RepoWithSeries(series string) *repo.CharmRepo {
	return repo.NewRepo(localCharmRepo, series)
}

// CharmRepo returns a new charm repository.
func CharmRepo() *repo.CharmRepo {
	return repo.NewRepo("charms", "")
}

// CheckCharmReady ensures that a desired charm archive exists and
// has some content.
func CheckCharmReady(c *tc.C, charmArchive *charm.CharmArchive) {
	fileSize := func() int64 {
		f, err := os.Open(charmArchive.Path)
		c.Assert(err, tc.ErrorIsNil)
		defer func() { _ = f.Close() }()

		fi, err := f.Stat()
		c.Assert(err, tc.ErrorIsNil)
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
	return os.WriteFile(pathToArchive, buf.Bytes(), 0644)
}
