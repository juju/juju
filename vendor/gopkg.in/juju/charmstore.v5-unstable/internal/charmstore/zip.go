// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"

import (
	"archive/zip"
	"compress/flate"
	"io"

	"gopkg.in/errgo.v1"

	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
)

// ZipFileReader returns a reader that will read
// content referred to by f within zipr, which should
// refer to the contents of a zip file,
func ZipFileReader(zipr io.ReadSeeker, f mongodoc.ZipFile) (io.Reader, error) {
	if _, err := zipr.Seek(f.Offset, 0); err != nil {
		return nil, errgo.Notef(err, "cannot seek to %d in zip content", f.Offset)
	}
	content := io.LimitReader(zipr, f.Size)
	if !f.Compressed {
		return content, nil
	}
	return flate.NewReader(content), nil
}

// NewZipFile returns a new mongodoc zip file
// reference to the given zip file.
func NewZipFile(f *zip.File) (mongodoc.ZipFile, error) {
	offset, err := f.DataOffset()
	if err != nil {
		return mongodoc.ZipFile{}, errgo.Notef(err, "cannot determine data offset for %q", f.Name)
	}
	zf := mongodoc.ZipFile{
		Offset: offset,
		Size:   int64(f.CompressedSize64),
	}
	switch f.Method {
	case zip.Store:
	case zip.Deflate:
		zf.Compressed = true
	default:
		return mongodoc.ZipFile{}, errgo.Newf("unknown zip compression method for %q", f.Name)
	}
	return zf, nil
}
