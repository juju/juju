// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/juju/internal/storage"
)

// noopFilesystemSource is an implementation of storage.FilesystemSource
// that does nothing.
//
// noopFilesystemSource is expected to be called from a single goroutine.
type noopFilesystemSource struct{}

// ValidateFilesystemParams is defined on storage.FilesystemSource.
func (s *noopFilesystemSource) ValidateFilesystemParams(params storage.FilesystemParams) error {
	return nil
}

// CreateFilesystems is defined on storage.FilesystemSource.
func (s *noopFilesystemSource) CreateFilesystems(_ context.Context, args []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error) {
	return nil, nil
}

// DestroyFilesystems is defined on storage.FilesystemSource.
func (s *noopFilesystemSource) DestroyFilesystems(_ context.Context, filesystemIds []string) ([]error, error) {
	return make([]error, len(filesystemIds)), nil
}

// ReleaseFilesystems is defined on storage.FilesystemSource.
func (s *noopFilesystemSource) ReleaseFilesystems(_ context.Context, filesystemIds []string) ([]error, error) {
	return make([]error, len(filesystemIds)), nil
}

// AttachFilesystems is defined on storage.FilesystemSource.
func (s *noopFilesystemSource) AttachFilesystems(_ context.Context, args []storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error) {
	return nil, nil
}

// DetachFilesystems is defined on storage.FilesystemSource.
func (s *noopFilesystemSource) DetachFilesystems(_ context.Context, args []storage.FilesystemAttachmentParams) ([]error, error) {
	return make([]error, len(args)), nil
}
