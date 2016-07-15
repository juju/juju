// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v5"

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// serveContent serves the given content as a single HTTP endpoint.
// We use http.FileServer under the covers because that
// provides us with all the HTTP Content-Range goodness
// that we'd like.
// TODO use http.ServeContent instead of this.
func serveContent(w http.ResponseWriter, req *http.Request, length int64, content io.ReadSeeker) {
	fs := &archiveFS{
		length:     length,
		ReadSeeker: content,
	}
	// Copy the request and mutate the path to pretend
	// we're looking for the given file.
	nreq := *req
	nreq.URL.Path = "/archive.zip"
	h := http.FileServer(fs)
	h.ServeHTTP(w, &nreq)
}

// archiveFS implements http.FileSystem to serve a single file.
// http.FileSystem.Open returns an http.File; http.File.Stat returns an
// os.FileInfo. We implement methods for all of those interfaces on the
// same type, and return the same value for all the aforementioned
// methods, since we only ever need one instance of any of them.
type archiveFS struct {
	length int64
	io.ReadSeeker
}

// Open implements http.FileSystem.Open.
func (fs *archiveFS) Open(name string) (http.File, error) {
	if name != "/archive.zip" {
		return nil, fmt.Errorf("unexpected name %q", name)
	}
	return fs, nil
}

// Close implements http.File.Close.
// It does not actually close anything because
// that responsibility is left to the caller of serveContent.
func (fs *archiveFS) Close() error {
	return nil
}

// Stat implements http.File.Stat.
func (fs *archiveFS) Stat() (os.FileInfo, error) {
	return fs, nil
}

// Readdir implements http.File.Readdir.
func (fs *archiveFS) Readdir(count int) ([]os.FileInfo, error) {
	return nil, fmt.Errorf("not a directory")
}

// Name implements os.FileInfo.Name.
func (fs *archiveFS) Name() string {
	return "archive"
}

// Size implements os.FileInfo.Size.
func (fs *archiveFS) Size() int64 {
	return fs.length
}

// Mode implements os.FileInfo.Mode.
func (fs *archiveFS) Mode() os.FileMode {
	return 0444
}

// ModTime implements os.FileInfo.ModTime.
func (fs *archiveFS) ModTime() time.Time {
	return time.Time{}
}

// IsDir implements os.FileInfo.IsDir.
func (fs *archiveFS) IsDir() bool {
	return false
}

// Sys implements os.FileInfo.Sys.
func (fs *archiveFS) Sys() interface{} {
	return nil
}
