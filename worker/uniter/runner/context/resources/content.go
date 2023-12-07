// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"bytes"
	"io"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"
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

// Checker provides the functionality for verifying that read data
// is correct.
type Checker struct {
	// Content holds the expected content values.
	content     Content
	checksum    *charmresource.FingerprintHash
	sizeTracker *utils.SizeTracker
}

// NewContentChecker returns a Checker for the provided data.
func NewContentChecker(content Content) *Checker {
	var sizeTracker utils.SizeTracker
	return &Checker{
		content:     content,
		checksum:    charmresource.NewFingerprintHash(),
		sizeTracker: &sizeTracker,
	}
}

// WrapReader implements ContentChecker.
func (c Checker) WrapReader(reader io.Reader) io.Reader {
	hashingReader := io.TeeReader(reader, c.checksum)
	return io.TeeReader(hashingReader, c.sizeTracker)
}

// Verify implements ContentChecker.
func (c Checker) Verify() error {
	size := c.sizeTracker.Size()
	fp := c.checksum.Fingerprint()
	if err := c.content.Verify(size, fp); err != nil {
		return errors.Trace(err)
	}
	return nil
}
