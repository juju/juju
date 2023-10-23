// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
)

// FilesystemSource is an implementation of storage.FilesystemSource, suitable for
// testing. Each method's default behaviour may be overridden by setting
// the corresponding Func field.
type FilesystemSource struct {
	testing.Stub

	CreateFilesystemsFunc        func(envcontext.ProviderCallContext, []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error)
	DestroyFilesystemsFunc       func(envcontext.ProviderCallContext, []string) ([]error, error)
	ReleaseFilesystemsFunc       func(envcontext.ProviderCallContext, []string) ([]error, error)
	ValidateFilesystemParamsFunc func(storage.FilesystemParams) error
	AttachFilesystemsFunc        func(envcontext.ProviderCallContext, []storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error)
	DetachFilesystemsFunc        func(envcontext.ProviderCallContext, []storage.FilesystemAttachmentParams) ([]error, error)
}

// CreateFilesystems is defined on storage.FilesystemSource.
func (s *FilesystemSource) CreateFilesystems(ctx envcontext.ProviderCallContext, params []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error) {
	s.MethodCall(s, "CreateFilesystems", ctx, params)
	if s.CreateFilesystemsFunc != nil {
		return s.CreateFilesystemsFunc(ctx, params)
	}
	return nil, errors.NotImplementedf("CreateFilesystems")
}

// DestroyFilesystems is defined on storage.FilesystemSource.
func (s *FilesystemSource) DestroyFilesystems(ctx envcontext.ProviderCallContext, volIds []string) ([]error, error) {
	s.MethodCall(s, "DestroyFilesystems", ctx, volIds)
	if s.DestroyFilesystemsFunc != nil {
		return s.DestroyFilesystemsFunc(ctx, volIds)
	}
	return nil, errors.NotImplementedf("DestroyFilesystems")
}

// ReleaseFilesystems is defined on storage.FilesystemSource.
func (s *FilesystemSource) ReleaseFilesystems(ctx envcontext.ProviderCallContext, volIds []string) ([]error, error) {
	s.MethodCall(s, "ReleaseFilesystems", ctx, volIds)
	if s.ReleaseFilesystemsFunc != nil {
		return s.ReleaseFilesystemsFunc(ctx, volIds)
	}
	return nil, errors.NotImplementedf("ReleaseFilesystems")
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
func (s *FilesystemSource) AttachFilesystems(ctx envcontext.ProviderCallContext, params []storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error) {
	s.MethodCall(s, "AttachFilesystems", ctx, params)
	if s.AttachFilesystemsFunc != nil {
		return s.AttachFilesystemsFunc(ctx, params)
	}
	return nil, errors.NotImplementedf("AttachFilesystems")
}

// DetachFilesystems is defined on storage.FilesystemSource.
func (s *FilesystemSource) DetachFilesystems(ctx envcontext.ProviderCallContext, params []storage.FilesystemAttachmentParams) ([]error, error) {
	s.MethodCall(s, "DetachFilesystems", ctx, params)
	if s.DetachFilesystemsFunc != nil {
		return s.DetachFilesystemsFunc(ctx, params)
	}
	return nil, errors.NotImplementedf("DetachFilesystems")
}
