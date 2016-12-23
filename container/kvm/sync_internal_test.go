// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux
// +build amd64 arm64 ppc64el

package kvm

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/environs/imagedownloads"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

// syncInternalSuite is gocheck boilerplate.
type syncInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&syncInternalSuite{})

const imageContents = "fake img file"

func (syncInternalSuite) TestFetcher(c *gc.C) {
	ts := newTestServer()
	defer ts.Close()

	md := newTestMetadata(ts.URL)

	tmpdir, pathfinder, ok := newTmpdir()
	if !ok {
		c.Fatal("failed to setup temp dir in test")
	}
	defer func() {
		err := os.RemoveAll(tmpdir)
		if err != nil {
			c.Errorf("got error %q when removing tmpdir %q",
				err.Error(),
				tmpdir)
		}
	}()

	fetcher, err := newDefaultFetcher(md, pathfinder)
	c.Assert(err, jc.ErrorIsNil)

	// setup a fake command runner.
	stub := runStub{}
	fetcher.image.runCmd = stub.Run

	err = fetcher.Fetch()
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(fetcher.image.tmpFile.Name())
	c.Check(os.IsNotExist(err), jc.IsTrue)

	// Check that our call was made as expected.
	c.Assert(stub.Calls(), gc.HasLen, 1)
	c.Assert(stub.Calls()[0], gc.Matches, "qemu-img convert -f qcow2 .*/juju-kvm-server.img-.* .*/guests/spammy-archless-backing-file.qcow")

}

func (syncInternalSuite) TestFetcherWriteFails(c *gc.C) {
	ts := newTestServer()
	defer ts.Close()

	md := newTestMetadata(ts.URL)

	tmpdir, pathfinder, ok := newTmpdir()
	if !ok {
		c.Fatal("failed to setup temp dir in test")
	}
	defer func() {
		err := os.RemoveAll(tmpdir)
		if err != nil {
			c.Errorf("got error %q when removing tmpdir %q",
				err.Error(),
				tmpdir)
		}
	}()

	fetcher, err := newDefaultFetcher(md, pathfinder)
	c.Assert(err, jc.ErrorIsNil)

	// setup a fake command runner.
	stub := runStub{err: errors.Errorf("boom")}
	fetcher.image.runCmd = stub.Run

	// Make sure we got the error.
	err = fetcher.Fetch()
	c.Assert(err, gc.ErrorMatches, "boom")

	// Check that our call was made as expected.
	c.Assert(stub.Calls(), gc.HasLen, 1)
	c.Assert(stub.Calls()[0], gc.Matches, "qemu-img convert -f qcow2 .*/juju-kvm-server.img-.* .*/guests/spammy-archless-backing-file.qcow")

}

func (syncInternalSuite) TestFetcherInvalidSHA(c *gc.C) {
	ts := newTestServer()
	defer ts.Close()

	md := newTestMetadata(ts.URL)
	md.SHA256 = "invalid"

	tmpdir, pathfinder, ok := newTmpdir()
	if !ok {
		c.Fatal("failed to setup temp dir in test")
	}
	defer func() {
		err := os.RemoveAll(tmpdir)
		if err != nil {
			c.Errorf("got error %q when removing tmpdir %q",
				err.Error(),
				tmpdir)
		}
	}()

	fetcher, err := newDefaultFetcher(md, pathfinder)
	c.Assert(err, jc.ErrorIsNil)

	err = fetcher.Fetch()
	c.Assert(err, gc.ErrorMatches, "hash sum mismatch for /tmp/juju-kvm-.*")
}

func (syncInternalSuite) TestFetcherNotFound(c *gc.C) {
	ts := newTestServer()
	defer ts.Close()

	md := newTestMetadata(ts.URL)
	md.Path = "not-there"

	tmpdir, pathfinder, ok := newTmpdir()
	if !ok {
		c.Fatal("failed to setup temp dir in test")
	}
	defer func() {
		err := os.RemoveAll(tmpdir)
		if err != nil {
			c.Errorf("got error %q when removing tmpdir %q",
				err.Error(),
				tmpdir)
		}
	}()

	fetcher, err := newDefaultFetcher(md, pathfinder)
	c.Assert(err, jc.ErrorIsNil)

	err = fetcher.Fetch()
	c.Check(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, `got 404 fetching image "not-there" not found`)
}

func newTestMetadata(base string) *imagedownloads.Metadata {
	return &imagedownloads.Metadata{
		Arch:    "archless",
		Release: "spammy",
		Version: "version",
		FType:   "ftype",
		SHA256:  "5e8467e6732923e74de52ef60134ba747aeeb283812c60f69b67f4f79aca1475",
		Path:    "server.img",
		Size:    int64(len(imageContents)),
		BaseURL: base,
	}
}

func newTestServer() *httptest.Server {
	mtime := time.Unix(1000, 0).UTC()
	imageFile := &fakeFileInfo{
		basename: "series-image.img",
		modtime:  mtime,
		contents: imageContents,
	}
	fs := fakeFS{
		"/": &fakeFileInfo{
			dir:  true,
			ents: []*fakeFileInfo{imageFile},
		},
		"/server.img": imageFile,
	}
	return httptest.NewServer(http.FileServer(fs))
}

// newTmpdir creates a tmpdir and returns pathfinder func that returns the
// tmpdir.
func newTmpdir() (string, func(string) (string, error), bool) {
	td, err := ioutil.TempDir("", "juju-test-kvm-internalSuite")
	if err != nil {
		return "", nil, false
	}
	pathfinder := func(string) (string, error) { return td, nil }
	return td, pathfinder, true
}

type fakeFileInfo struct {
	dir      bool
	basename string
	modtime  time.Time
	ents     []*fakeFileInfo
	contents string
	err      error
}

func (f *fakeFileInfo) Name() string       { return f.basename }
func (f *fakeFileInfo) Sys() interface{}   { return nil }
func (f *fakeFileInfo) ModTime() time.Time { return f.modtime }
func (f *fakeFileInfo) IsDir() bool        { return f.dir }
func (f *fakeFileInfo) Size() int64        { return int64(len(f.contents)) }
func (f *fakeFileInfo) Mode() os.FileMode {
	if f.dir {
		return 0755 | os.ModeDir
	}
	return 0644
}

type fakeFile struct {
	io.ReadSeeker
	fi     *fakeFileInfo
	path   string // as opened
	entpos int
}

func (f *fakeFile) Close() error               { return nil }
func (f *fakeFile) Stat() (os.FileInfo, error) { return f.fi, nil }
func (f *fakeFile) Readdir(count int) ([]os.FileInfo, error) {
	if !f.fi.dir {
		return nil, os.ErrInvalid
	}
	var fis []os.FileInfo

	limit := f.entpos + count
	if count <= 0 || limit > len(f.fi.ents) {
		limit = len(f.fi.ents)
	}
	for ; f.entpos < limit; f.entpos++ {
		fis = append(fis, f.fi.ents[f.entpos])
	}

	if len(fis) == 0 && count > 0 {
		return fis, io.EOF
	}
	return fis, nil
}

type fakeFS map[string]*fakeFileInfo

func (fs fakeFS) Open(name string) (http.File, error) {
	name = path.Clean(name)
	f, ok := fs[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	if f.err != nil {
		return nil, f.err
	}
	return &fakeFile{ReadSeeker: strings.NewReader(f.contents), fi: f, path: name}, nil
}
