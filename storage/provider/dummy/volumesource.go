package dummy

import (
	"github.com/juju/errors"
	"github.com/juju/juju/storage"
	"github.com/juju/testing"
)

// VolumeSource is an implementation of storage.VolumeSource, suitable for
// testing. Each method's default behaviour may be overridden by setting
// the corresponding Func field.
type VolumeSource struct {
	testing.Stub

	CreateVolumesFunc        func([]storage.VolumeParams) ([]storage.Volume, []storage.VolumeAttachment, error)
	ListVolumesFunc          func() ([]string, error)
	DescribeVolumesFunc      func([]string) ([]storage.VolumeInfo, error)
	DestroyVolumesFunc       func([]string) []error
	ValidateVolumeParamsFunc func(storage.VolumeParams) error
	AttachVolumesFunc        func([]storage.VolumeAttachmentParams) ([]storage.VolumeAttachment, error)
	DetachVolumesFunc        func([]storage.VolumeAttachmentParams) error
}

// CreateVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) CreateVolumes(params []storage.VolumeParams) ([]storage.Volume, []storage.VolumeAttachment, error) {
	s.MethodCall(s, "CreateVolumes", params)
	if s.CreateVolumesFunc != nil {
		return s.CreateVolumesFunc(params)
	}
	return nil, nil, errors.NotImplementedf("CreateVolumes")
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
func (s *VolumeSource) DescribeVolumes(volIds []string) ([]storage.VolumeInfo, error) {
	s.MethodCall(s, "DescribeVolumes", volIds)
	if s.DescribeVolumesFunc != nil {
		return s.DescribeVolumesFunc(volIds)
	}
	return nil, errors.NotImplementedf("DescribeVolumes")
}

// DestroyVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) DestroyVolumes(volIds []string) []error {
	s.MethodCall(s, "DestroyVolumes", volIds)
	if s.DestroyVolumesFunc != nil {
		return s.DestroyVolumesFunc(volIds)
	}
	errs := make([]error, len(volIds))
	for i := range errs {
		errs[i] = errors.NotImplementedf("DestroyVolumes")
	}
	return errs
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
func (s *VolumeSource) AttachVolumes(params []storage.VolumeAttachmentParams) ([]storage.VolumeAttachment, error) {
	s.MethodCall(s, "AttachVolumes", params)
	if s.AttachVolumesFunc != nil {
		return s.AttachVolumesFunc(params)
	}
	return nil, errors.NotImplementedf("AttachVolumes")
}

// DetachVolumes is defined on storage.VolumeSource.
func (s *VolumeSource) DetachVolumes(params []storage.VolumeAttachmentParams) error {
	s.MethodCall(s, "DetachVolumes", params)
	if s.DetachVolumesFunc != nil {
		return s.DetachVolumesFunc(params)
	}
	return errors.NotImplementedf("DetachVolumes")

}
