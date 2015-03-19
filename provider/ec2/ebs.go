// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

const (
	EBS_ProviderType = storage.ProviderType("ebs")

	// Config attributes
	// TODO(wallyworld) - use juju/schema for defining attributes

	// The volume type (default standard):
	//   "gp2" for General Purpose (SSD) volumes
	//   "io1" for Provisioned IOPS (SSD) volumes,
	//   "standard" for Magnetic volumes.
	EBS_VolumeType = "volume-type" // top level directory where loop devices are created.

	// The number of I/O operations per second (IOPS) to provision for the volume.
	// Only valid for Provisioned IOPS (SSD) volumes.
	EBS_IOPS = "iops" // optional subdirectory for loop devices.

	// Specifies whether the volume should be encrypted.
	EBS_Encrypted = "encrypted"
)

func init() {
	ebsssdPool, _ := storage.NewConfig("ebs-ssd", EBS_ProviderType, map[string]interface{}{"volume-type": "gp2"})
	defaultPools := []*storage.Config{
		ebsssdPool,
	}
	poolmanager.RegisterDefaultStoragePools(defaultPools)
}

// loopProviders create volume sources which use loop devices.
type ebsProvider struct{}

var _ storage.Provider = (*ebsProvider)(nil)

var validConfigOptions = set.NewStrings(
	storage.Persistent,
	EBS_VolumeType,
	EBS_IOPS,
	EBS_Encrypted,
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

// Supports is defined on the Provider interface.
func (e *ebsProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindBlock
}

// Scope is defined on the Provider interface.
func (e *ebsProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic is defined on the Provider interface.
func (e *ebsProvider) Dynamic() bool {
	// TODO(axw) this should be changed to true when support for dynamic
	// provisioning has been implemented for EBS. At that point, we need
	// to remove the block device mapping code.
	return false
}

func TranslateUserEBSOptions(userOptions map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range userOptions {
		if k == EBS_VolumeType {
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

// FilesystemSource is defined on the Provider interface.
func (e *ebsProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type ebsVolumeSource struct {
}

var _ storage.VolumeSource = (*ebsVolumeSource)(nil)

func (v *ebsVolumeSource) CreateVolumes([]storage.VolumeParams) ([]storage.Volume, []storage.VolumeAttachment, error) {
	panic("not implemented")
}

func (v *ebsVolumeSource) DescribeVolumes(volIds []string) ([]storage.Volume, error) {
	panic("not implemented")
}

func (v *ebsVolumeSource) DestroyVolumes(volIds []string) []error {
	panic("not implemented")
}

func (v *ebsVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	panic("not implemented")
}

func (v *ebsVolumeSource) AttachVolumes([]storage.VolumeAttachmentParams) ([]storage.VolumeAttachment, error) {
	panic("not implemented")
}

func (v *ebsVolumeSource) DetachVolumes([]storage.VolumeAttachmentParams) error {
	panic("not implemented")
}
