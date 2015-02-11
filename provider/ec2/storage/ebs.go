// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

const (
	EBSProviderType = storage.ProviderType("ebs")

	// Config attributes

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

	// The Availability Zone in which to create the volume.
	availabilityZone = "availability-zone"
)

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
	if _, ok := providerConfig.Attrs()[availabilityZone]; ok {
		return errors.Errorf(
			"%q cannot be specified as a pool option as it needs to match the deployed instance", availabilityZone,
		)
	}
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

type ebsVolueSoucre struct {
}

var _ storage.VolumeSource = (*ebsVolueSoucre)(nil)

func (v *ebsVolueSoucre) CreateVolumes([]storage.VolumeParams) ([]storage.Volume, []storage.VolumeAttachment, error) {
	panic("not implemented")
}

func (v *ebsVolueSoucre) DescribeVolumes(volIds []string) ([]storage.Volume, error) {
	panic("not implemented")
}

func (v *ebsVolueSoucre) DestroyVolumes(volIds []string) error {
	panic("not implemented")
}

func (v *ebsVolueSoucre) ValidateVolumeParams(params storage.VolumeParams) error {
	panic("not implemented")
}

func (v *ebsVolueSoucre) AttachVolumes([]storage.VolumeAttachmentParams) ([]storage.VolumeAttachment, error) {
	panic("not implemented")
}

func (v *ebsVolueSoucre) DetachVolumes([]storage.VolumeAttachmentParams) error {
	panic("not implemented")
}
