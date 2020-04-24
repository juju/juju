// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"bytes"
	"io"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"

	"github.com/juju/juju/resource"
)

// Content holds a reader for the content of a resource along
// with details about that content.
type Content struct {
	// Data holds the resource content, ready to be read (once).
	Data io.Reader

	// Size is the byte count of the data.
	Size int64

	// Fingerprint holds the checksum of the data.
	Fingerprint charmresource.Fingerprint
}

// Verify ensures that the actual resource content details match
// the expected ones.
func (c Content) Verify(size int64, fp charmresource.Fingerprint) error {
	if size != c.Size {
		return errors.Errorf("resource size does not match expected (%d != %d)", size, c.Size)
	}
	// Only verify a finger print if it's set - older clients did not send a
	// fingerprint for docker resources.
	if c.Fingerprint.IsZero() {
		return nil
	}
	if !bytes.Equal(fp.Bytes(), c.Fingerprint.Bytes()) {
		return errors.Errorf("resource fingerprint does not match expected (%q != %q)", fp, c.Fingerprint)
	}
	return nil
}

// ContentSource represents the functionality of OpenedResource,
// relative to Content.
type ContentSource interface {
	// Content returns the content for the opened resource.
	Content() Content

	// Info returns the info for the opened resource.
	Info() resource.Resource
}

// TODO(ericsnow) Need a lockfile around create/write?

// WriteContent writes the resource file to the target provided
// by the deps.
func WriteContent(target io.Writer, content Content, deps WriteContentDeps) error {
	checker := deps.NewChecker(content)
	source := checker.WrapReader(content.Data)

	if err := deps.Copy(target, source); err != nil {
		return errors.Annotate(err, "could not write resource to file")
	}

	if err := checker.Verify(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// WriteContentDeps exposes the external functionality needed by WriteContent.
type WriteContentDeps interface {
	//NewChecker provides a content checker for the given content.
	NewChecker(Content) ContentChecker

	// Copy copies the data from the reader into the writer.
	Copy(io.Writer, io.Reader) error
}

// ContentChecker exposes functionality for verifying the data read from a reader.
type ContentChecker interface {
	// WrapReader wraps the provided reader in another reader
	// that tracks the read data.
	WrapReader(io.Reader) io.Reader

	// Verify fails if the tracked data does not match
	// the expected data.
	Verify() error
}

// Checker provides the functionality for verifying that read data
// is correct.
type Checker struct {
	// Content holds the expected content values.
	Content Content

	// SizeTracker tracks the number of bytes read.
	SizeTracker SizeTracker

	// ChecksumWriter tracks the checksum of the read bytes.
	ChecksumWriter ChecksumWriter
}

// NewContentChecker returns a Checker for the provided data.
func NewContentChecker(content Content, sizeTracker SizeTracker, checksumWriter ChecksumWriter) *Checker {
	return &Checker{
		Content:        content,
		SizeTracker:    sizeTracker,
		ChecksumWriter: checksumWriter,
	}
}

// WrapReader implements ContentChecker.
func (c Checker) WrapReader(reader io.Reader) io.Reader {
	hashingReader := io.TeeReader(reader, c.ChecksumWriter)
	return io.TeeReader(hashingReader, c.SizeTracker)
}

// Verify implements ContentChecker.
func (c Checker) Verify() error {
	size := c.SizeTracker.Size()
	fp := c.ChecksumWriter.Fingerprint()
	if err := c.Content.Verify(size, fp); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// NopChecker is a ContentChecker that accepts all data.
type NopChecker struct{}

// WrapReader implements ContentChecker.
func (NopChecker) WrapReader(reader io.Reader) io.Reader {
	return reader
}

// Verify implements ContentChecker.
func (NopChecker) Verify() error {
	return nil
}

// SizeTracker tracks the number of bytes written.
type SizeTracker interface {
	io.Writer

	// Size returns the number of bytes written.
	Size() int64
}

// ChecksumWriter tracks the checksum of all written bytes.
type ChecksumWriter interface {
	io.Writer

	// Fingerprint is the fingerprint for the tracked checksum.
	Fingerprint() charmresource.Fingerprint
}
