// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/collections/set"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// Attrs defines storage attributes.
type Attrs map[string]string

// StoragePool represents a storage pool in Juju.
// It contains the name of the pool, the provider type, and any attributes
type StoragePool struct {
	UUID     string
	Name     string
	Provider string
	Attrs    Attrs
	OriginID int
}

// These type aliases are used to specify filter terms.
type (
	Names     []string
	Providers []string
)

func deduplicateNamesOrProviders[T ~[]string](namesOrProviders T) T {
	if len(namesOrProviders) == 0 {
		return nil
	}
	// Ensure uniqueness and no empty values.
	result := set.NewStrings()
	for _, v := range namesOrProviders {
		if v != "" {
			result.Add(v)
		}
	}
	if result.IsEmpty() {
		return nil
	}
	return T(result.Values())
}

// Values returns the unique values of the Names.
func (n Names) Values() []string {
	return deduplicateNamesOrProviders(n)
}

// Values returns the unique values of the Providers.
func (p Providers) Values() []string {
	return deduplicateNamesOrProviders(p)
}

// FilesystemInfo describes information about a filesystem.
type FilesystemInfo struct {
	storage.FilesystemInfo
	Pool          string
	BackingVolume *storage.VolumeInfo
}

// RecommendedStoragePoolParams represents a recommended storage pool assignment
// at the service layer boundary. It is accepted by services and translated into
// state-layer arguments before being persisted.
type RecommendedStoragePoolParams struct {
	StoragePoolUUID StoragePoolUUID
	StorageKind     StorageKind
}

// ImportStoragePoolParams represents a storage pool definition used when importing
// storage pools into the model.
type ImportStoragePoolParams struct {
	UUID   StoragePoolUUID
	Name   string
	Origin StoragePoolOrigin
	Type   string
	Attrs  map[string]any
}

// ImportStorageInstanceParams represents data to import a storage instance
// and its owner.
type ImportStorageInstanceParams struct {
	StorageName       string
	StorageKind       string
	StorageInstanceID string
	RequestedSizeMiB  uint64
	PoolName          string
	UnitName          string
	AttachedUnitNames []string
}

// Validate returns NotValid if the params have an empty StorageID or
// PoolName or RequestedSizeMiB.
func (i ImportStorageInstanceParams) Validate() error {
	if i.PoolName == "" || i.RequestedSizeMiB == 0 || i.StorageInstanceID == "" {
		return errors.New("empty PoolName, RequestedSizeMiB, or StorageInstanceID not valid").Add(coreerrors.NotValid)
	}

	if i.UnitName != "" {
		if err := coreunit.Name(i.UnitName).Validate(); err != nil {
			return err
		}
	}

	for _, attachment := range i.AttachedUnitNames {
		if err := coreunit.Name(attachment).Validate(); err != nil {
			return err
		}
	}

	if !IsValidStoragePoolNameWithLegacy(i.PoolName) {
		return errors.Errorf("invalid PoolName %q", i.PoolName).Add(coreerrors.NotValid)
	}

	return nil
}

// ImportFilesystemParams represents data to import a filesystem.
type ImportFilesystemParams struct {
	ID                string
	SizeInMiB         uint64
	ProviderID        string
	PoolName          string
	StorageInstanceID string
	Attachments       []ImportFilesystemAttachmentsParams
}

// Validate returns NotValid if the params are not valid
func (p ImportFilesystemParams) Validate() error {
	if p.ID == "" {
		return errors.Errorf("empty ID not valid").Add(coreerrors.NotValid)
	}

	if !IsValidStoragePoolNameWithLegacy(p.PoolName) {
		return errors.Errorf("storage pool name %q not valid", p.PoolName).Add(coreerrors.NotValid)
	}

	if p.StorageInstanceID != "" {
		if err := corestorage.ID(p.StorageInstanceID).Validate(); err != nil {
			return errors.Errorf("storage instance ID %q: %w", p.StorageInstanceID, err).Add(coreerrors.NotValid)
		}
	}

	for i, attachment := range p.Attachments {
		if err := attachment.Validate(); err != nil {
			return errors.Errorf("invalid attachment %d: %w", i, err).Add(coreerrors.NotValid)
		}
	}

	return nil
}

// ImportFilesystemAttachmentsParams represents data to import filesystem
// attachments.
type ImportFilesystemAttachmentsParams struct {
	HostMachineName string
	HostUnitName    string
	MountPoint      string
	ProviderID      string
	ReadOnly        bool
}

func (p ImportFilesystemAttachmentsParams) Validate() error {
	if p.HostMachineName == "" && p.HostUnitName == "" {
		return errors.New("either HostMachineName or HostUnitName must be provided").Add(coreerrors.NotValid)
	}
	if p.HostUnitName != "" && p.HostMachineName != "" {
		return errors.New("only one of HostMachineName or HostUnitName can be provided").Add(coreerrors.NotValid)
	}

	if p.HostUnitName != "" {
		if err := coreunit.Name(p.HostUnitName).Validate(); err != nil {
			return err
		}
	}

	if p.HostMachineName != "" {
		if err := coremachine.Name(p.HostMachineName).Validate(); err != nil {
			return err
		}
	}

	if p.MountPoint == "" {
		return errors.New("MountPoint cannot be empty").Add(coreerrors.NotValid)
	}

	return nil
}

// ImportVolumeParams represents a volume definition used when importing
// volumes into the model.
type ImportVolumeParams struct {
	ID                string
	StorageInstanceID string
	Provisioned       bool
	SizeMiB           uint64
	Pool              string
	HardwareID        string
	WWN               string
	ProviderID        string
	Persistent        bool
	Attachments       []ImportVolumeAttachmentParams
	AttachmentPlans   []ImportVolumeAttachmentPlanParams
}

// Validate returns an NotValid error if the ImportVolumeParams does not
// contain an ID, StorageInstanceID, SizeMiB, nor storage pool.
func (i ImportVolumeParams) Validate() error {
	if i.ID == "" {
		return errors.New("empty volume ID not valid").Add(coreerrors.NotValid)
	}

	if i.SizeMiB == 0 {
		return errors.Errorf("empty size for volume %q not valid", i.ID).Add(coreerrors.NotValid)
	}

	if !IsValidStoragePoolNameWithLegacy(i.Pool) {
		return errors.Errorf("storage pool name %q not valid", i.Pool).Add(coreerrors.NotValid)
	}

	if i.StorageInstanceID != "" {
		if err := corestorage.ID(i.StorageInstanceID).Validate(); err != nil {
			return errors.Errorf("storage ID %q: %w", i.StorageInstanceID, err).Add(coreerrors.NotValid)
		}
	}

	for _, a := range i.Attachments {
		if err := a.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// StoragePoolNameUUID represents a storage pool name and uuid pair.
type StoragePoolNameUUID struct {
	Name string `db:"name"`
	UUID string `db:"uuid"`
}

// ImportVolumeAttachmentParams represents a volume attachment used when
// importing volumes into the model.
type ImportVolumeAttachmentParams struct {
	HostMachineName string
	HostUnitName    string
	Provisioned     bool
	ReadOnly        bool
	DeviceName      string
	DeviceLink      string
	BusAddress      string
}

// Validate returns an NotValid if ImportVolumeAttachmentParams does not meet
// following criteria:
// 1. HostMachineName must be valid.
// 2. If provisioned, DeviceName and DeviceLink must be set.
func (i ImportVolumeAttachmentParams) Validate() error {
	// Volumes can only be attached to machines.
	if err := coremachine.Name(i.HostMachineName).Validate(); err != nil {
		return err
	}

	if i.Provisioned {
		if i.DeviceName == "" || i.DeviceLink == "" {
			return errors.New("a provisioned attachment with empty device name and device link not valid").Add(coreerrors.NotValid)
		}
	}
	return nil
}

// CoreBlockDevice returns a coreblockdevice.BlockDevice representation of
// the ImportVolumeAttachmentParams. Use to match with existing block devices.
func (i ImportVolumeAttachmentParams) CoreBlockDevice() coreblockdevice.BlockDevice {
	deviceLinks := make([]string, 0)
	if i.DeviceLink != "" {
		deviceLinks = append(deviceLinks, i.DeviceLink)
	}
	return coreblockdevice.BlockDevice{
		DeviceName:  i.DeviceName,
		DeviceLinks: deviceLinks,
		BusAddress:  i.BusAddress,
	}
}

// ImportVolumeAttachmentPlanParams represents a volume attachment plan used when
// importing volumes into the model.
type ImportVolumeAttachmentPlanParams struct {
	HostMachineName  string
	DeviceType       string
	DeviceAttributes map[string]string
}

// Validate returns a NotValid error if ImportVolumeAttachmentPlanParams does
// not meet following criteria:
// 1. HostMachineName must be valid.
// 2. DeviceType must be a valid volume device type.
func (i ImportVolumeAttachmentPlanParams) Validate() error {
	if err := coremachine.Name(i.HostMachineName).Validate(); err != nil {
		return err
	}

	var err error
	if i.DeviceType != "" {
		_, err = ParseVolumeDeviceType(i.DeviceType)
	}

	return err
}

// UserStoragePoolParams represents the user storage pools data from an incoming model.
type UserStoragePoolParams struct {
	Name       string
	Provider   string
	Attributes map[string]interface{}
}
