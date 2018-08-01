// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"io"
	"sort"

	"github.com/juju/errors"
)

// SizeReaderAt combines io.ReaderAt with a Size method.
type SizeReaderAt interface {
	// Size returns the size of the data readable
	// from the reader.
	Size() int64
	io.ReaderAt
}

// NewMultiReaderAt is like io.MultiReader but produces a ReaderAt
// (and Size), instead of just a reader.
//
// Note: this implementation was taken from a talk given
// by Brad Fitzpatrick as OSCON 2013.
//
// http://talks.golang.org/2013/oscon-dl.slide#49
// https://github.com/golang/talks/blob/master/2013/oscon-dl/server-compose.go
func NewMultiReaderAt(parts ...SizeReaderAt) SizeReaderAt {
	m := &multiReaderAt{
		parts: make([]offsetAndSource, 0, len(parts)),
	}
	var off int64
	for _, p := range parts {
		m.parts = append(m.parts, offsetAndSource{off, p})
		off += p.Size()
	}
	m.size = off
	return m
}

type offsetAndSource struct {
	off int64
	SizeReaderAt
}

type multiReaderAt struct {
	parts []offsetAndSource
	size  int64
}

func (m *multiReaderAt) Size() int64 {
	return m.size
}

func (m *multiReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	wantN := len(p)

	// Skip past the requested offset.
	skipParts := sort.Search(len(m.parts), func(i int) bool {
		// This function returns whether parts[i] will
		// contribute any bytes to our output.
		part := m.parts[i]
		return part.off+part.Size() > off
	})
	parts := m.parts[skipParts:]

	// How far to skip in the first part.
	needSkip := off
	if len(parts) > 0 {
		needSkip -= parts[0].off
	}

	for len(parts) > 0 && len(p) > 0 {
		readP := p
		partSize := parts[0].Size()
		if int64(len(readP)) > partSize-needSkip {
			readP = readP[:partSize-needSkip]
		}
		pn, err0 := parts[0].ReadAt(readP, needSkip)
		if err0 != nil {
			return n, err0
		}
		n += pn
		p = p[pn:]
		if int64(pn)+needSkip == partSize {
			parts = parts[1:]
		}
		needSkip = 0
	}

	if n != wantN {
		err = io.ErrUnexpectedEOF
	}
	return
}

// NewMultiReaderSeeker returns an io.ReadSeeker that combines
// all the given readers into a single one. It assumes that
// all the seekers are initially positioned at the start.
func NewMultiReaderSeeker(readers ...io.ReadSeeker) io.ReadSeeker {
	sreaders := make([]SizeReaderAt, len(readers))
	for i, r := range readers {
		r1, err := newSizeReaderAt(r)
		if err != nil {
			panic(err)
		}
		sreaders[i] = r1
	}
	return &readSeeker{
		r: NewMultiReaderAt(sreaders...),
	}
}

// newSizeReaderAt adapts an io.ReadSeeker to a SizeReaderAt.
// Note that it doesn't strictly adhere to the ReaderAt
// contract because it's not safe to call ReadAt concurrently.
// This doesn't matter because io.ReadSeeker doesn't
// need to be thread-safe and this is only used in that
// context.
func newSizeReaderAt(r io.ReadSeeker) (SizeReaderAt, error) {
	size, err := r.Seek(0, 2)
	if err != nil {
		return nil, err
	}
	return &sizeReaderAt{
		r:    r,
		size: size,
		off:  size,
	}, nil
}

// sizeReaderAt adapts an io.ReadSeeker to a SizeReaderAt.
type sizeReaderAt struct {
	r    io.ReadSeeker
	size int64
	off  int64
}

// ReadAt implemnts SizeReaderAt.ReadAt.
func (r *sizeReaderAt) ReadAt(buf []byte, off int64) (n int, err error) {
	if off != r.off {
		_, err = r.r.Seek(off, 0)
		if err != nil {
			return 0, err
		}
		r.off = off
	}
	n, err = io.ReadFull(r.r, buf)
	r.off += int64(n)
	return n, err
}

// Size implemnts SizeReaderAt.Size.
func (r *sizeReaderAt) Size() int64 {
	return r.size
}

// readSeeker adapts a SizeReaderAt to an io.ReadSeeker.
type readSeeker struct {
	r   SizeReaderAt
	off int64
}

// Seek implements io.Seeker.Seek.
func (r *readSeeker) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case 0:
	case 1:
		off += r.off
	case 2:
		off = r.r.Size() + off
	}
	if off < 0 {
		return 0, errors.New("negative position")
	}
	r.off = off
	return off, nil
}

// Read implements io.Reader.Read.
func (r *readSeeker) Read(buf []byte) (int, error) {
	n, err := r.r.ReadAt(buf, r.off)
	r.off += int64(n)
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n, err
}
