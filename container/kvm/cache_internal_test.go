// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
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

	"github.com/juju/juju/environs/imagedownloads"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

// internalCacheSuite is gocheck boilerplate.
type internalCacheSuite struct{}

var _ = gc.Suite(internalCacheSuite{})

const imageContents = "fake img file"

func (internalCacheSuite) TestUpdater(c *gc.C) {
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
	ts := httptest.NewServer(http.FileServer(fs))
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

	updater, err := newDefaultUpdater(md, pathfinder)
	c.Assert(err, jc.ErrorIsNil)

	// Test 200 returns reader, nil
	err = updater.Update()
	c.Assert(err, jc.ErrorIsNil)

	// Test 304 returns nil,nil and that mtimes don't change.
	finfo, err := os.Stat(tmpdir + "/kvm/images/" + md.Path)
	if err != nil {
		c.Fatalf("failed to stat expected file: %s", err.Error())
	}
	pinfo, err := os.Stat(tmpdir + "/kvm/metadata/" + md.Path)
	if err != nil {
		c.Fatalf("failed to stat expected file: %s", err.Error())
	}

	updater, err = newDefaultUpdater(md, pathfinder)
	err = updater.Update()
	c.Assert(err, jc.ErrorIsNil)

	finfo2, err := os.Stat(tmpdir + "/kvm/images/" + md.Path)
	if err != nil {
		c.Fatalf("failed to stat expected file: %s", err.Error())
	}
	pinfo2, err := os.Stat(tmpdir + "/kvm/metadata/" + md.Path)
	if err != nil {
		c.Fatalf("failed to stat expected file: %s", err.Error())
	}

	c.Check(finfo.ModTime(), jc.DeepEquals, finfo2.ModTime())
	c.Check(pinfo.ModTime(), jc.DeepEquals, pinfo2.ModTime())

	// Test setting the time forward on the upstream image to test updating an
	// existing image. Also ensure the mtime on the files changes.
	time.Sleep(5 * time.Millisecond) // need enough time to flush the changes to disk.

	imageFile.modtime = imageFile.modtime.Add(1 * time.Hour)
	updater, err = newDefaultUpdater(md, pathfinder)
	updater.fileCache.ModTime = mtime.Format(http.TimeFormat)
	err = updater.Update()
	c.Assert(err, jc.ErrorIsNil)

	finfo2, err = os.Stat(tmpdir + "/kvm/images/" + md.Path)
	if err != nil {
		c.Fatalf("failed to stat expected file: %s", err.Error())
	}
	pinfo2, err = os.Stat(tmpdir + "/kvm/metadata/" + md.Path)
	if err != nil {
		c.Fatalf("failed to stat expected file: %s", err.Error())
	}
	c.Check(finfo2.ModTime().Sub(finfo.ModTime()), jc.GreaterThan, 0)
	c.Check(pinfo2.ModTime().Sub(pinfo.ModTime()), jc.GreaterThan, 0)

	// anything else (404, dns issue, etc...)  should return nil, err
	md.Path = "notthere"
	updater, err = newDefaultUpdater(md, pathfinder)
	err = updater.Update()
	c.Assert(err, gc.ErrorMatches, `got "404 Not Found" fetching kvm image URL "http.*/notthere"`)
}

func newTestMetadata(base string) *imagedownloads.Metadata {
	return &imagedownloads.Metadata{
		Arch:    "test-arch",
		Release: "test-release",
		Version: "test-version",
		FType:   "test-ftype",
		SHA256:  "test-sha256",
		Path:    "server.img",
		Size:    int64(len(imageContents)),
		BaseURL: base,
	}
}

// newTmpdir creates a tmpdir and returns pathfinder func that returns the tmpdir.
func newTmpdir() (string, func(string) (string, error), bool) {
	td, err := ioutil.TempDir("", "juju-test-internalCacheSuite")
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
