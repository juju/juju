// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/juju/storage"
)

type volumeSource struct{}

var _ storage.VolumeSource = (*volumeSource)(nil)

func (v *volumeSource) CreateVolumes(params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	return nil, nil
}

func (v *volumeSource) ListVolumes() ([]string, error) {
	return nil, nil
}

func (v *volumeSource) DescribeVolumes(volIds []string) ([]storage.DescribeVolumesResult, error) {
	return nil, nil
}

func (v *volumeSource) DestroyVolumes(volIds []string) ([]error, error) {
	return nil, nil
}

func (v *volumeSource) ReleaseVolumes(volIds []string) ([]error, error) {
	return nil, nil
}

func (v *volumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	return nil
}

func (v *volumeSource) AttachVolumes(params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	return nil, nil
}

func (v *volumeSource) DetachVolumes(params []storage.VolumeAttachmentParams) ([]error, error) {
	return nil, nil
}
