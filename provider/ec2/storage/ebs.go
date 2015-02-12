// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/pool"
)

const (
	EBSProviderType = storage.ProviderType("ebs")

	// Config attributes
	// TODO(wallyworld) - use juju/schema for defining attributes

	// The volume type (default standard):
	//   "gp2" for General Purpose (SSD) volumes
	//   "io1" for Provisioned IOPS (SSD) volumes,
	//   "standard" for Magnetic volumes.
	VolumeType = "volume-type" // top level directory where loop devices are created.

	// The number of I/O operations per second (IOPS) to provision for the volume.
	// Only valid for Provisioned IOPS (SSD) volumes.
	IOPS = "iops" // optional subdirectory for loop devices.

	// Specifies whether the volume should be encrypted.
	Encrypted = "encrypted"
)

func init() {
	defaultPools := []pool.PoolInfo{
		// TODO(wallyworld) - remove "ebs" pool which has no params when we support
		// specifying provider type for pool name
		{"ebs", EBSProviderType, map[string]interface{}{}},
		{"ebs-ssd", EBSProviderType, map[string]interface{}{"volume-type": "gp2"}},
	}
	pool.RegisterDefaultStoragePools(defaultPools)
}

// loopProviders create volume sources which use loop devices.
type ebsProvider struct{}

var _ storage.Provider = (*ebsProvider)(nil)

var validConfigOptions = set.NewStrings(
	VolumeType,
	IOPS,
	Encrypted,
)

// ValidateConfig is defined on the Provider interface.
func (e *ebsProvider) ValidateConfig(providerConfig *storage.Config) error {
	// TODO - check valid values as well as attr names
	for attr := range providerConfig.Attrs() {
		if !validConfigOptions.Contains(attr) {
			return errors.Errorf("unknown provider config option %q", attr)
		}
	}
	return nil
}

func TranslateUserEBSOptions(userOptions map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range userOptions {
		if k == VolumeType {
			switch v {
			case "magnetic":
				v = "standard"
			case "ssd":
				v = "gp2"
			case "provisioned-iops":
				v = "io1"
			}
		}
		result[k] = v
	}
	return result
}

// VolumeSource is defined on the Provider interface.
func (e *ebsProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	panic("not implemented")
}

type ebsVolumeSoucre struct {
}

var _ storage.VolumeSource = (*ebsVolumeSoucre)(nil)

func (v *ebsVolumeSoucre) CreateVolumes([]storage.VolumeParams) ([]storage.Volume, []storage.VolumeAttachment, error) {
	panic("not implemented")
}

func (v *ebsVolumeSoucre) DescribeVolumes(volIds []string) ([]storage.Volume, error) {
	panic("not implemented")
}

func (v *ebsVolumeSoucre) DestroyVolumes(volIds []string) error {
	panic("not implemented")
}

func (v *ebsVolumeSoucre) ValidateVolumeParams(params storage.VolumeParams) error {
	panic("not implemented")
}

func (v *ebsVolumeSoucre) AttachVolumes([]storage.VolumeAttachmentParams) ([]storage.VolumeAttachment, error) {
	panic("not implemented")
}

func (v *ebsVolumeSoucre) DetachVolumes([]storage.VolumeAttachmentParams) error {
	panic("not implemented")
}
