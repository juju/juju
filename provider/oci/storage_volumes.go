// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

type volumeSource struct{}

var _ storage.VolumeSource = (*volumeSource)(nil)

func (v *volumeSource) CreateVolumes(params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	return nil, errors.NotImplementedf("CreateVolumes")
}

func (v *volumeSource) ListVolumes() ([]string, error) {
	return nil, errors.NotImplementedf("ListVolumes")
}

func (v *volumeSource) DescribeVolumes(volIds []string) ([]storage.DescribeVolumesResult, error) {
	return nil, errors.NotImplementedf("DescribeVolumes")
}

func (v *volumeSource) DestroyVolumes(volIds []string) ([]error, error) {
	return nil, errors.NotImplementedf("DestroyVolumes")
}

func (v *volumeSource) ReleaseVolumes(volIds []string) ([]error, error) {
	return nil, errors.NotImplementedf("ReleaseVolumes")
}

func (v *volumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	return errors.NotImplementedf("ValidateVolumeParams")
}

func (v *volumeSource) AttachVolumes(params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	return nil, errors.NotImplementedf("AttachVolumes")
}

func (v *volumeSource) DetachVolumes(params []storage.VolumeAttachmentParams) ([]error, error) {
	return nil, errors.NotImplementedf("DetachVolumes")
}
