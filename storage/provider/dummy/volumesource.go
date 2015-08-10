package dummy

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/storage"
)

// VolumeSource is an implementation of storage.VolumeSource, suitable for
// testing. Each method's default behaviour may be overridden by setting
// the corresponding Func field.
type VolumeSource struct {
	testing.Stub

	CreateVolumesFunc        func([]storage.VolumeParams) ([]storage.CreateVolumesResult, error)
	ListVolumesFunc          func() ([]string, error)
	DescribeVolumesFunc      func([]string) ([]storage.DescribeVolumesResult, error)
	DestroyVolumesFunc       func([]string) ([]error, error)
	ValidateVolumeParamsFunc func(storage.VolumeParams) error
	AttachVolumesFunc        func([]storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error)
	DetachVolumesFunc        func([]storage.VolumeAttachmentParams) ([]error, error)
}

// CreateVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) CreateVolumes(params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	s.MethodCall(s, "CreateVolumes", params)
	if s.CreateVolumesFunc != nil {
		return s.CreateVolumesFunc(params)
	}
	return nil, errors.NotImplementedf("CreateVolumes")
}

// ListVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) ListVolumes() ([]string, error) {
	s.MethodCall(s, "ListVolumes")
	if s.ListVolumesFunc != nil {
		return s.ListVolumesFunc()
	}
	return nil, nil
}

// DescribeVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) DescribeVolumes(volIds []string) ([]storage.DescribeVolumesResult, error) {
	s.MethodCall(s, "DescribeVolumes", volIds)
	if s.DescribeVolumesFunc != nil {
		return s.DescribeVolumesFunc(volIds)
	}
	return nil, errors.NotImplementedf("DescribeVolumes")
}

// DestroyVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) DestroyVolumes(volIds []string) ([]error, error) {
	s.MethodCall(s, "DestroyVolumes", volIds)
	if s.DestroyVolumesFunc != nil {
		return s.DestroyVolumesFunc(volIds)
	}
	return nil, errors.NotImplementedf("DestroyVolumes")
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
func (s *VolumeSource) AttachVolumes(params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	s.MethodCall(s, "AttachVolumes", params)
	if s.AttachVolumesFunc != nil {
		return s.AttachVolumesFunc(params)
	}
	return nil, errors.NotImplementedf("AttachVolumes")
}

// DetachVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) DetachVolumes(params []storage.VolumeAttachmentParams) ([]error, error) {
	s.MethodCall(s, "DetachVolumes", params)
	if s.DetachVolumesFunc != nil {
		return s.DetachVolumesFunc(params)
	}
	return nil, errors.NotImplementedf("DetachVolumes")
}
