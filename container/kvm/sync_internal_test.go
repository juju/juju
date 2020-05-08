// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagedownloads"
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

	md := newTestMetadata()

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

	fetcher, err := newDefaultFetcher(md, ts.URL, pathfinder, nil)
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
	c.Assert(stub.Calls()[0], gc.Matches, " qemu-img convert -f qcow2 .*/juju-kvm-server.img-.* .*/guests/spammy-archless-backing-file.qcow")

}

func (syncInternalSuite) TestFetcherWriteFails(c *gc.C) {
	ts := newTestServer()
	defer ts.Close()

	md := newTestMetadata()

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

	fetcher, err := newDefaultFetcher(md, ts.URL, pathfinder, nil)
	c.Assert(err, jc.ErrorIsNil)

	// setup a fake command runner.
	stub := runStub{err: errors.Errorf("boom")}
	fetcher.image.runCmd = stub.Run

	// Make sure we got the error.
	err = fetcher.Fetch()
	c.Assert(err, gc.ErrorMatches, "boom")

	// Check that our call was made as expected.
	c.Assert(stub.Calls(), gc.HasLen, 1)
	c.Assert(stub.Calls()[0], gc.Matches, " qemu-img convert -f qcow2 .*/juju-kvm-server.img-.* .*/guests/spammy-archless-backing-file.qcow")

}

func (syncInternalSuite) TestFetcherInvalidSHA(c *gc.C) {
	ts := newTestServer()
	defer ts.Close()

	md := newTestMetadata()
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

	fetcher, err := newDefaultFetcher(md, ts.URL, pathfinder, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = fetcher.Fetch()
	c.Assert(err, gc.ErrorMatches, "hash sum mismatch for /tmp/juju-kvm-.*")
}

func (syncInternalSuite) TestFetcherNotFound(c *gc.C) {
	ts := newTestServer()
	defer ts.Close()

	md := newTestMetadata()
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

	fetcher, err := newDefaultFetcher(md, ts.URL, pathfinder, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = fetcher.Fetch()
	c.Check(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, `got 404 fetching image "not-there" not found`)
}

func newTestMetadata() *imagedownloads.Metadata {
	return &imagedownloads.Metadata{
		Arch:    "archless",
		Release: "spammy",
		Version: "version",
		FType:   "ftype",
		SHA256:  "5e8467e6732923e74de52ef60134ba747aeeb283812c60f69b67f4f79aca1475",
		Path:    "server.img",
		Size:    int64(len(imageContents)),
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

type progressWriterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&progressWriterSuite{})

func (s *progressWriterSuite) TestOnlyPercentChanges(c *gc.C) {
	cbLog := []string{}
	loggingCB := func(msg string) {
		cbLog = append(cbLog, msg)
	}
	clock := testclock.NewClock(time.Date(2007, 1, 1, 10, 20, 30, 1234, time.UTC))
	// We are using clock to actually measure time, not trigger an event, which
	// causes the testclock.Clock to think we're doing something wrong, so we
	// just create one waiter that we'll otherwise ignore.
	ignored := clock.After(10 * time.Second)
	_ = ignored
	writer := progressWriter{
		callback: loggingCB,
		url:      "http://host/path",
		total:    0,
		maxBytes: 100 * 1024 * 1024, // 100 MB
		clock:    clock,
	}
	content := make([]byte, 50*1024)
	// Start the clock before the first tick, that way every tick represents
	// exactly 1ms and 50kiB written.
	now := clock.Now()
	writer.startTime = &now
	for i := 0; i < 2048; i++ {
		clock.Advance(time.Millisecond)
		n, err := writer.Write(content)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(n, gc.Equals, len(content))
	}
	expectedCB := []string{}
	for i := 1; i <= 100; i++ {
		// We tick every 1ms and add 50kiB each time, which is
		// 50*1024 *1000/ 1000/1000  = 51MB/s
		expectedCB = append(expectedCB, fmt.Sprintf("copying http://host/path %d%% (51MB/s)", i))
	}
	// There are 2048 calls to Write, but there should only be 100 calls to progress update
	c.Check(len(cbLog), gc.Equals, 100)
	c.Check(cbLog, gc.DeepEquals, expectedCB)
}
