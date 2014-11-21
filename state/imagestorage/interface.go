// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagestorage

import (
	"io"
	"time"
)

// Metadata describes a image blob.
type Metadata struct {
	Series   string
	Arch     string
	Kind     string
	Size     int64
	Checksum string
	Created  time.Time
}

// Storage provides methods for storing and retrieving images by kind, series, and arch.
type Storage interface {
	// AddImage adds the image blob and metadata into state,
	// replacing existing metadata if any exists with the specified version.
	AddImage(io.Reader, *Metadata) error

	// DeleteImage deletes the image blob defined by metadata from state.
	DeleteImage(*Metadata) error

	// Image returns the Metadata and image blob contents
	// for the specified kind, series, arch if it exists, else an error
	// satisfying errors.IsNotFound.
	Image(kind, series, arch string) (*Metadata, io.ReadCloser, error)
}

// StorageCloser extends the Storage interface with a Close method.
type StorageCloser interface {
	Storage
	Close() error
}
