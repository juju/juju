// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagestorage

import (
	"io"
	"time"
)

// Metadata describes an image blob.
type Metadata struct {
	EnvUUID   string
	Series    string
	Arch      string
	Kind      string
	Size      int64
	SHA256    string
	Created   time.Time
	SourceURL string
}

// ImageFilter is used to query image metadata.
type ImageFilter struct {
	Kind   string
	Series string
	Arch   string
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

	// ListImages returns the image metadata matching the specified filter.
	ListImages(filter ImageFilter) ([]*Metadata, error)
}
