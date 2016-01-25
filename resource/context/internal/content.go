// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"bytes"
	"io"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

// Content holds a reader for the content of a resource along
// with details about that content.
type Content struct {
	// Data holds the resouce content, ready to be read (once).
	Data io.Reader

	// Size is the byte count of the data.
	Size int64

	// Fingerprint holds the checksum of the data.
	Fingerprint charmresource.Fingerprint
}

// verify ensures that the actual resource content details match
// the expected ones.
func (c Content) Verify(size int64, fp charmresource.Fingerprint) error {
	if size != c.Size {
		return errors.Errorf("resource size does not match expected (%d != %d)", size, c.Size)
	}
	if !bytes.Equal(fp.Bytes(), c.Fingerprint.Bytes()) {
		return errors.Errorf("resource fingerprint does not match expected (%q != %q)", fp, c.Fingerprint)
	}
	return nil
}

type ContentSource interface {
	// Content returns the content for the opened resource.
	Content() Content

	// Info returns the info for the opened resource.
	Info() resource.Resource
}

// TODO(ericsnow) Need a lockfile around create/write?

func WriteContent(content Content, deps WriteContentDeps) error {
	checker := deps.NewChecker(content)
	source := checker.WrapReader(content.Data)

	target, err := deps.CreateTarget()
	if err != nil {
		return errors.Annotate(err, "could not create new file for resource")
	}
	defer target.Close()
	// TODO(ericsnow) chmod 0644?

	if _, err := io.Copy(target, source); err != nil {
		return errors.Annotate(err, "could not write resource to file")
	}

	if err := checker.Verify(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type WriteContentDeps interface {
	NewChecker(Content) ContentChecker

	CreateTarget() (io.WriteCloser, error)
}

type ContentChecker interface {
	WrapReader(io.Reader) io.Reader

	Verify() error
}

type Checker struct {
	Content Content

	SizeTracker SizeTracker

	ChecksumWriter ChecksumWriter
}

func NewContentChecker(content Content, st SizeTracker, checksumWriter ChecksumWriter) *Checker {
	return &Checker{
		Content:        content,
		SizeTracker:    st,
		ChecksumWriter: checksumWriter,
	}
}

func (c Checker) WrapReader(reader io.Reader) io.Reader {
	hashingReader := io.TeeReader(reader, c.ChecksumWriter)
	return io.TeeReader(hashingReader, c.SizeTracker)
}

func (c Checker) Verify() error {
	size := c.SizeTracker.Size()
	fp := c.ChecksumWriter.Fingerprint()
	if err := c.Content.Verify(size, fp); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type NopChecker struct{}

func (NopChecker) WrapReader(reader io.Reader) io.Reader {
	return reader
}

func (NopChecker) Verify() error {
	return nil
}

type SizeTracker interface {
	io.Writer

	Size() int64
}

type ChecksumWriter interface {
	io.Writer

	Fingerprint() charmresource.Fingerprint
}
