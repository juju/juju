// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/storage"
)

// FilesystemSource is an implementation of storage.FilesystemSource, suitable for
// testing. Each method's default behaviour may be overridden by setting
// the corresponding Func field.
type FilesystemSource struct {
	testing.Stub

	CreateFilesystemsFunc        func([]storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error)
	DestroyFilesystemsFunc       func([]string) ([]error, error)
	ValidateFilesystemParamsFunc func(storage.FilesystemParams) error
	AttachFilesystemsFunc        func([]storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error)
	DetachFilesystemsFunc        func([]storage.FilesystemAttachmentParams) ([]error, error)
}

// CreateFilesystems is defined on storage.FilesystemSource.
func (s *FilesystemSource) CreateFilesystems(params []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error) {
	s.MethodCall(s, "CreateFilesystems", params)
	if s.CreateFilesystemsFunc != nil {
		return s.CreateFilesystemsFunc(params)
	}
	return nil, errors.NotImplementedf("CreateFilesystems")
}

// DestroyFilesystems is defined on storage.FilesystemSource.
func (s *FilesystemSource) DestroyFilesystems(volIds []string) ([]error, error) {
	s.MethodCall(s, "DestroyFilesystems", volIds)
	if s.DestroyFilesystemsFunc != nil {
		return s.DestroyFilesystemsFunc(volIds)
	}
	return nil, errors.NotImplementedf("DestroyFilesystems")
}

// ValidateFilesystemParams is defined on storage.FilesystemSource.
func (s *FilesystemSource) ValidateFilesystemParams(params storage.FilesystemParams) error {
	s.MethodCall(s, "ValidateFilesystemParams", params)
	if s.ValidateFilesystemParamsFunc != nil {
		return s.ValidateFilesystemParamsFunc(params)
	}
	return nil
}

// AttachFilesystems is defined on storage.FilesystemSource.
func (s *FilesystemSource) AttachFilesystems(params []storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error) {
	s.MethodCall(s, "AttachFilesystems", params)
	if s.AttachFilesystemsFunc != nil {
		return s.AttachFilesystemsFunc(params)
	}
	return nil, errors.NotImplementedf("AttachFilesystems")
}

// DetachFilesystems is defined on storage.FilesystemSource.
func (s *FilesystemSource) DetachFilesystems(params []storage.FilesystemAttachmentParams) ([]error, error) {
	s.MethodCall(s, "DetachFilesystems", params)
	if s.DetachFilesystemsFunc != nil {
		return s.DetachFilesystemsFunc(params)
	}
	return nil, errors.NotImplementedf("DetachFilesystems")
}
