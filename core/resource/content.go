// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

// TODO(ericsnow) Move this file to the charm repo?

import (
	"io"
	"os"

	"github.com/juju/utils/v4"

	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
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

// GenerateContent returns a new Content for the given data stream.
func GenerateContent(reader io.ReadSeeker) (Content, error) {
	var sizer utils.SizeTracker
	sizingReader := io.TeeReader(reader, &sizer)
	fp, err := charmresource.GenerateFingerprint(sizingReader)
	if err != nil {
		return Content{}, errors.Capture(err)
	}
	if _, err := reader.Seek(0, os.SEEK_SET); err != nil {
		return Content{}, errors.Capture(err)
	}
	size := sizer.Size()

	content := Content{
		Data:        reader,
		Size:        size,
		Fingerprint: fp,
	}
	return content, nil
}
