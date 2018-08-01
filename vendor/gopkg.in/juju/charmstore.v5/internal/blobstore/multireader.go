// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blobstore // import "gopkg.in/juju/charmstore.v5/internal/blobstore"

import (
	"io"
	"sort"

	"gopkg.in/errgo.v1"

	"gopkg.in/juju/charmstore.v5/internal/mongodoc"
)

// TODO use equivalent constants in io package when we can use more
// modern Go version.
const (
	seekStart   = 0 // seek relative to the origin of the file
	seekCurrent = 1 // seek relative to the current offset
	seekEnd     = 2 // seek relative to the end
)

type multiReader struct {
	// store holds the underlying store.
	store *Store

	// endPos holds the absolute position of the end
	// of each part.
	endPos []int64

	// hashes holds the hash of each part.
	hashes []string

	// size holds the size the entire multipart blob.
	size int64

	// r holds the currently open blob.
	r ReadSeekCloser

	// rindex holds the index of the currently open blob.
	rindex int

	// rpos holds the current position that the reader
	// is at.
	rpos int64

	// pos holds the current seek position. This may be
	// different from rpos when Seek has been called.
	pos int64
}

func newMultiReader(store *Store, idx *mongodoc.MultipartIndex) (ReadSeekCloser, int64, error) {
	if len(idx.Sizes) != len(idx.Hashes) {
		return nil, 0, errgo.Newf("index size/length mismatch (database corruption?)")
	}
	switch len(idx.Sizes) {
	case 0:
		return emptyBlob{}, 0, nil
	case 1:
		// No point in going through the multireader logic if there's
		// only one part.
		return store.Open(idx.Hashes[0], nil)
	}
	endPos := make([]int64, len(idx.Sizes))
	p := int64(0)
	for i, size := range idx.Sizes {
		p += int64(size)
		endPos[i] = p
	}
	return &multiReader{
		store:  store,
		endPos: endPos,
		hashes: idx.Hashes,
		size:   p,
	}, p, nil
}

func (r *multiReader) Close() error {
	if r.r != nil {
		return r.r.Close()
	}
	return nil
}

// Read implements io.Reader.Read.
func (r *multiReader) Read(buf []byte) (int, error) {
	if r.pos >= r.size {
		return 0, io.EOF
	}
	if r.r == nil || r.pos >= r.endPos[r.rindex] || r.pos < r.startPos(r.rindex) {
		if r.r != nil {
			r.r.Close()
			r.r = nil
		}
		// Binary search for the index of the earliest position
		// that ends after the current part. Note that because
		// we know that 0 <= r.pos < r.size and the last element
		// in r.endPos==r.size, we must end up with 0 <=
		// r.rindex < len(r.endPos).
		r.rindex = sort.Search(len(r.endPos), func(i int) bool {
			return r.endPos[i] > r.pos
		})
		nr, _, err := r.store.Open(r.hashes[r.rindex], nil)
		if err != nil {
			return 0, errgo.Notef(err, "cannot open blob part")
		}
		r.r = nr
		r.rpos = r.startPos(r.rindex)
	}
	if r.pos != r.rpos {
		_, err := r.r.Seek(r.pos-r.startPos(r.rindex), seekStart)
		if err != nil {
			return 0, errgo.Notef(err, "cannot seek into blob")
		}
		r.rpos = r.pos
	}
	n, err := r.r.Read(buf)
	r.pos += int64(n)
	r.rpos = r.pos
	if err != nil {
		return n, errgo.Notef(err, "error reading blob %q", r.hashes[r.rindex])
	}
	return n, nil
}

// startPos returns the start position of given part.
func (r *multiReader) startPos(part int) int64 {
	if part == 0 {
		return 0
	}
	return r.endPos[part-1]
}

// Seek implements io.Seeker.Seek.
func (r *multiReader) Seek(pos int64, whence int) (int64, error) {
	switch whence {
	case seekStart:
	case seekEnd:
		pos = r.size - pos
	case seekCurrent:
		pos = r.pos + pos
	default:
		return 0, errgo.Newf("unknown seek whence value")
	}
	if pos < 0 {
		pos = 0
	}
	r.pos = pos
	return pos, nil
}

type emptyBlob struct{}

func (emptyBlob) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (emptyBlob) Seek(pos int64, whence int) (int64, error) {
	return 0, nil
}

func (emptyBlob) Close() error {
	return nil
}
