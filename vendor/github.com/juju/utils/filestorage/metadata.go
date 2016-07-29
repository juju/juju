// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filestorage

import (
	"time"

	"github.com/juju/errors"
)

// RawDoc is a basic, uniquely identifiable document.
type RawDoc struct {
	// ID is the unique identifier for the document.
	ID string
}

// Doc wraps a document in the Document interface.
type Doc struct {
	Raw RawDoc
}

// ID returns the document's unique identifier.
func (d *Doc) ID() string {
	return d.Raw.ID
}

// SetID sets the document's unique identifier.  If the ID is already
// set, SetID() returns true (false otherwise).
func (d *Doc) SetID(id string) bool {
	if d.Raw.ID != "" {
		return true
	}
	d.Raw.ID = id
	return false
}

// RawFileMetadata holds info specific to stored files.
type RawFileMetadata struct {
	// Size is the size (in bytes) of the stored file.
	Size int64
	// Checksum is the checksum of the stored file.
	Checksum string
	// ChecksumFormat describes the kind of the checksum.
	ChecksumFormat string
	// Stored records the timestamp of when the file was last stored.
	Stored *time.Time
}

// FileMetadata contains the metadata for a single stored file.
type FileMetadata struct {
	Doc
	Raw RawFileMetadata
}

// NewMetadata returns a new Metadata for a stored file.
func NewMetadata() *FileMetadata {
	meta := FileMetadata{}
	return &meta
}

func (m *FileMetadata) Size() int64 {
	return m.Raw.Size
}

func (m *FileMetadata) Checksum() string {
	return m.Raw.Checksum
}

func (m *FileMetadata) ChecksumFormat() string {
	return m.Raw.ChecksumFormat
}

func (m *FileMetadata) Stored() *time.Time {
	return m.Raw.Stored
}

func (m *FileMetadata) SetFileInfo(size int64, checksum, format string) error {
	// Fall back to existing values.
	if size == 0 {
		size = m.Raw.Size
	}
	if checksum == "" {
		checksum = m.Raw.Checksum
	}
	if format == "" {
		format = m.Raw.ChecksumFormat
	}
	if checksum != "" {
		if format == "" {
			return errors.Errorf("missing checksum format")
		}
	} else if format != "" {
		return errors.Errorf("missing checksum")
	}
	// Only allow setting once.
	if m.Raw.Size != 0 && size != m.Raw.Size {
		return errors.Errorf("file information (size) already set")
	}
	if m.Raw.Checksum != "" && checksum != m.Raw.Checksum {
		return errors.Errorf("file information (checksum) already set")
	}
	if m.Raw.ChecksumFormat != "" && format != m.Raw.ChecksumFormat {
		return errors.Errorf("file information (checksum format) already set")
	}
	// Set the values.
	m.Raw.Size = size
	m.Raw.Checksum = checksum
	m.Raw.ChecksumFormat = format
	return nil
}

func (m *FileMetadata) SetStored(timestamp *time.Time) {
	if timestamp == nil {
		now := time.Now().UTC()
		m.Raw.Stored = &now
	} else {
		m.Raw.Stored = timestamp
	}
}
