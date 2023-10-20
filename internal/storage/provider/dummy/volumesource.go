// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
)

// VolumeSource is an implementation of storage.VolumeSource, suitable for
// testing. Each method's default behaviour may be overridden by setting
// the corresponding Func field.
type VolumeSource struct {
	testing.Stub

	CreateVolumesFunc        func(envcontext.ProviderCallContext, []storage.VolumeParams) ([]storage.CreateVolumesResult, error)
	ListVolumesFunc          func(envcontext.ProviderCallContext) ([]string, error)
	DescribeVolumesFunc      func(envcontext.ProviderCallContext, []string) ([]storage.DescribeVolumesResult, error)
	DestroyVolumesFunc       func(envcontext.ProviderCallContext, []string) ([]error, error)
	ReleaseVolumesFunc       func(envcontext.ProviderCallContext, []string) ([]error, error)
	ValidateVolumeParamsFunc func(storage.VolumeParams) error
	AttachVolumesFunc        func(envcontext.ProviderCallContext, []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error)
	DetachVolumesFunc        func(envcontext.ProviderCallContext, []storage.VolumeAttachmentParams) ([]error, error)
}

// CreateVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) CreateVolumes(ctx envcontext.ProviderCallContext, params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	s.MethodCall(s, "CreateVolumes", ctx, params)
	if s.CreateVolumesFunc != nil {
		return s.CreateVolumesFunc(ctx, params)
	}
	return nil, errors.NotImplementedf("CreateVolumes")
}

// ListVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) ListVolumes(ctx envcontext.ProviderCallContext) ([]string, error) {
	s.MethodCall(s, "ListVolumes", ctx)
	if s.ListVolumesFunc != nil {
		return s.ListVolumesFunc(ctx)
	}
	return nil, nil
}

// DescribeVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) DescribeVolumes(ctx envcontext.ProviderCallContext, volIds []string) ([]storage.DescribeVolumesResult, error) {
	s.MethodCall(s, "DescribeVolumes", ctx, volIds)
	if s.DescribeVolumesFunc != nil {
		return s.DescribeVolumesFunc(ctx, volIds)
	}
	return nil, errors.NotImplementedf("DescribeVolumes")
}

// DestroyVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) DestroyVolumes(ctx envcontext.ProviderCallContext, volIds []string) ([]error, error) {
	s.MethodCall(s, "DestroyVolumes", ctx, volIds)
	if s.DestroyVolumesFunc != nil {
		return s.DestroyVolumesFunc(ctx, volIds)
	}
	return nil, errors.NotImplementedf("DestroyVolumes")
}

// ReleaseVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) ReleaseVolumes(ctx envcontext.ProviderCallContext, volIds []string) ([]error, error) {
	s.MethodCall(s, "ReleaseVolumes", ctx, volIds)
	if s.ReleaseVolumesFunc != nil {
		return s.ReleaseVolumesFunc(ctx, volIds)
	}
	return nil, errors.NotImplementedf("ReleaseVolumes")
}

// ValidateVolumeParams is defined on storage.VolumeSource.
func (s *VolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	s.MethodCall(s, "ValidateVolumeParams", params)
	if s.ValidateVolumeParamsFunc != nil {
		return s.ValidateVolumeParamsFunc(params)
	}
	return nil
}

// AttachVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) AttachVolumes(ctx envcontext.ProviderCallContext, params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	s.MethodCall(s, "AttachVolumes", ctx, params)
	if s.AttachVolumesFunc != nil {
		return s.AttachVolumesFunc(ctx, params)
	}
	return nil, errors.NotImplementedf("AttachVolumes")
}

// DetachVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) DetachVolumes(ctx envcontext.ProviderCallContext, params []storage.VolumeAttachmentParams) ([]error, error) {
	s.MethodCall(s, "DetachVolumes", ctx, params)
	if s.DetachVolumesFunc != nil {
		return s.DetachVolumesFunc(ctx, params)
	}
	return nil, errors.NotImplementedf("DetachVolumes")
}
