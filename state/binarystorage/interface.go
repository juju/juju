// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package binarystorage

import (
	"context"
	"io"
)

// Metadata describes a binary file stored in the storage.
type Metadata struct {
	Version string
	Size    int64
	SHA256  string
}

// Storage provides methods for storing and retrieving binary files by version.
type Storage interface {
	// Add adds the binary file and metadata into state, replacing existing
	// metadata if any exists with the specified version.
	Add(context.Context, io.Reader, Metadata) error

	// Open returns the Metadata and binary file contents for the specified
	// version if it exists, else an error satisfying errors.IsNotFound.
	Open(ctx context.Context, version string) (Metadata, io.ReadCloser, error)

	// AllMetadata returns metadata for the full list of binary files in the
	// catalogue.
	AllMetadata() ([]Metadata, error)

	// Metadata returns the Metadata for the specified version if it exists,
	// else an error satisfying errors.IsNotFound.
	Metadata(version string) (Metadata, error)
}

// StorageCloser extends the Storage interface with a Close method.
type StorageCloser interface {
	Storage
	Close() error
}
