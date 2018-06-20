// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/storage"
)

type volumeSource struct{}

var _ storage.VolumeSource = (*volumeSource)(nil)

func (v *volumeSource) CreateVolumes(ctx context.ProviderCallContext, params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	return nil, errors.NotImplementedf("CreateVolumes")
}

func (v *volumeSource) ListVolumes(ctx context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotImplementedf("ListVolumes")
}

func (v *volumeSource) DescribeVolumes(ctx context.ProviderCallContext, volIds []string) ([]storage.DescribeVolumesResult, error) {
	return nil, errors.NotImplementedf("DescribeVolumes")
}

func (v *volumeSource) DestroyVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	return nil, errors.NotImplementedf("DestroyVolumes")
}

func (v *volumeSource) ReleaseVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	return nil, errors.NotImplementedf("ReleaseVolumes")
}

func (v *volumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	return errors.NotImplementedf("ValidateVolumeParams")
}

func (v *volumeSource) AttachVolumes(ctx context.ProviderCallContext, params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	return nil, errors.NotImplementedf("AttachVolumes")
}

func (v *volumeSource) DetachVolumes(ctx context.ProviderCallContext, params []storage.VolumeAttachmentParams) ([]error, error) {
	return nil, errors.NotImplementedf("DetachVolumes")
}
