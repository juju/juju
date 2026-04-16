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
	domainnetwork "github.com/juju/juju/domain/network"
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

// RecommendedStoragePoolArg represents a recommended storage pool assignment
// for the state layer to accept.
type RecommendedStoragePoolArg struct {
	StoragePoolUUID StoragePoolUUID
	StorageKind     StorageKind
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

// ProvisionScope declares to the model in what context a storage entity needs
// to be provisioned.
type ProvisionScope int

const (
	// ProvisionScopeModel indicates that the provisioner for the storage is to
	// be run within the context of the model.
	ProvisionScopeModel ProvisionScope = iota

	// ProvisionScopeMachine indicates that the provisioner for the storage is
	// to be run within the context of the machine.
	ProvisionScopeMachine
)

// AddUnitStorageOverride defines user overrides to change storage defaults
// used when adding new storage to a unit.
type AddUnitStorageOverride struct {
	// StoragePoolUUID is the storage pool UUID.
	StoragePoolUUID *StoragePoolUUID

	// SizeMiB is the size of the storage instance, in MiB.
	SizeMiB *uint64
}

// UnitAddStorageArg represents the arguments required to add storage to a
// unit. This will instantiate the instances and attachments for the unit.
type UnitAddStorageArg struct {
	// StorageInstances defines the new storage instances that must be created
	// for the unit.
	StorageInstances []CreateUnitStorageInstanceArg

	// StorageToAttach defines the storage instances that should be attached to
	// the unit. New storage instances defined in
	// [CreateUnitStorageArg.StorageInstances] are not automatically attached to
	// the unit and should be included in this list.
	StorageToAttach []CreateUnitStorageAttachmentArg

	// StorageToOwn defines the storage instances that should be owned by the
	// unit.
	StorageToOwn []StorageInstanceUUID

	// CountLessThanEqual is the maximum storage count allowed at the time the
	// add is performed in order for the add operation to be successful.
	CountLessThanEqual uint32
}

// IAASUnitAddStorageArg represents the arguments required for making storage
// for an IAAS unit. This complements [UnitAddStorageArg], allowing for an
// IAAS unit to augment storage that is destined for a machine.
type IAASUnitAddStorageArg struct {
	UnitAddStorageArg

	// FilesystemsToOwn defines filesystems that will be owned by the unit's
	// machine.
	FilesystemsToOwn []FilesystemUUID

	// VolumesToOwn defines volumes that will be owned by the unit's machine.
	VolumesToOwn []VolumeUUID
}

// CreateUnitStorageArg represents the arguments required for making storage
// for a unit. This will create and set the unit's storage directives and then
// instantiate the instances and attachments for the unit.
type CreateUnitStorageArg struct {
	// StorageDirectives defines the storage directives that should be created
	// for the unit.
	StorageDirectives []DirectiveArg

	// StorageInstances defines the new storage instances that must be created
	// for the unit.
	StorageInstances []CreateUnitStorageInstanceArg

	// StorageToAttach defines the storage instances that should be attached to
	// the unit. New storage instances defined in
	// [CreateUnitStorageArg.StorageInstances] are not automatically attached to
	// the unit and should be included in this list.
	StorageToAttach []CreateUnitStorageAttachmentArg

	// StorageToOwn defines the storage instances that should be owned by the
	// unit.
	StorageToOwn []StorageInstanceUUID
}

// CreateIAASUnitStorageArg represents the arguments required for making
// storage for an IAAS unit. This complements [CreateUnitStorageArg], allowing
// for an IAAS unit to augment storage that is destined for a machine.
type CreateIAASUnitStorageArg struct {
	// FilesystemsToOwn defines filesystems that will be owned by the unit's
	// machine.
	FilesystemsToOwn []FilesystemUUID

	// VolumesToOwn defines volumes that will be owned by the unit's machine.
	VolumesToOwn []VolumeUUID
}

// DirectiveArg describes the arguments required for a storage directive.
type DirectiveArg struct {
	// Count represents the number of storage instances that should be made for
	// this directive.
	Count uint32

	// Name relates to the charm storage name definition and must match up.
	Name Name

	// PoolUUID defines the storage pool UUID to use for the directive.
	PoolUUID StoragePoolUUID

	// Size defines the size of the storage directive in MiB.
	Size uint64
}

// CreateUnitStorageAttachmentArg describes the arguments required for creating
// a storage attachment.
type CreateUnitStorageAttachmentArg struct {
	// UUID is the unique identifier to associate with the storage attachment.
	UUID StorageAttachmentUUID

	// FilesystemAttachment describes a filesystem to attach for the storage
	// instance attachment.
	FilesystemAttachment *CreateUnitStorageFilesystemAttachmentArg

	// StorageInstanceUUID is the unique identifier of the storage instance to
	// attach to the unit.
	StorageInstanceUUID StorageInstanceUUID

	// VolumeAttachment describes a volume to attach for the storage instance
	// attachment.
	VolumeAttachment *CreateUnitStorageVolumeAttachmentArg
}

// CreateUnitStorageFilesystemArg describes a filesystem that should be
// created as part of a unit's storage.
type CreateUnitStorageFilesystemArg struct {
	// UUID describes the unique identifier of the filesystem to create
	// alongside the storage instance.
	UUID FilesystemUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created filesystem.
	ProvisionScope ProvisionScope
}

// CreateUnitStorageFilesystemAttachmentArg describes a filesystem attachment
// that should be created alongside a unit's storage in the model.
type CreateUnitStorageFilesystemAttachmentArg struct {
	// FilesystemUUID is the unique identifier of the filesystem to be
	// attached.
	FilesystemUUID FilesystemUUID

	// NetNodeUUID is the net node of the model entity that filesystem will be
	// attached to.
	NetNodeUUID domainnetwork.NetNodeUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created filesystem attachment.
	ProvisionScope ProvisionScope

	// UUID is the unique identifier to give the filesystem attachment in the
	// model.
	UUID FilesystemAttachmentUUID
}

// CreateUnitStorageInstanceArg describes a set of arguments that create a new
// storage instance on behalf of a unit.
type CreateUnitStorageInstanceArg struct {
	// CharmName is the name of the charm that this storage instance is being
	// provisioned for. This value helps Juju later identify what charm this
	// storage can be re-attached back to.
	CharmName string

	// Filesystem describes the properties of a new filesystem to be created
	// alongside the storage instance. If this value is not nil a new
	// filesystem will be created with the storage instance.
	Filesystem *CreateUnitStorageFilesystemArg

	// Kind defines the type of storage that is being created.
	Kind StorageKind

	// Name is the name of the storage and must correspond to the storage name
	// defined in the charm the unit is running.
	Name Name

	// RequestSizeMiB defines the requested size of this storage instance in
	// MiB. What ends up being allocated for the storage instance will be at
	// least this value.
	RequestSizeMiB uint64

	// StoragePoolUUID is the pool from which this storage instance is
	// provisioned.
	StoragePoolUUID StoragePoolUUID

	// Volume describes the properties of a new volume to be created alongside
	// the storage instance. If this value is not nil a new volume will be
	// created with the storage instance.
	Volume *CreateUnitStorageVolumeArg

	// UUID is the unique identifier to associate with the storage instance.
	UUID StorageInstanceUUID
}

// CreateUnitStorageVolumeArg describes a volume that should be created as
// part of a unit's storage.
type CreateUnitStorageVolumeArg struct {
	// UUID describes the unique identifier of the volume to create alongside
	// the storage instance.
	UUID VolumeUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created volume.
	ProvisionScope ProvisionScope
}

// CreateUnitStorageVolumeAttachmentArg describes a volume attachment that
// should be created alongside a unit's storage in the model.
type CreateUnitStorageVolumeAttachmentArg struct {
	// NetNodeUUID is the net node of the model entity that volume will be
	// attached to.
	NetNodeUUID domainnetwork.NetNodeUUID

	// ProvisionScope describes the provision scope to assign to the newly
	// created volume attachment.
	ProvisionScope ProvisionScope

	// VolumeUUID is the unique identifier of the volume to be attached.
	VolumeUUID VolumeUUID

	// UUID is the unique identifier to give the volume attachment in the
	// model.
	UUID VolumeAttachmentUUID

	// ProviderID, if set, forms the pre-determined volume attachment provider
	// ID.
	ProviderID *string
}

// RegisterUnitStorageArg represents the arguments required for registering a
// unit's storage that has appeared in the model. This allows re-using
// previously created storage for the unit and provisioning new storage as
// needed.
type RegisterUnitStorageArg struct {
	CreateUnitStorageArg

	// FilesystemProviderIDs defines the provider ID value to set for each
	// filesystem.
	FilesystemProviderIDs map[FilesystemUUID]string

	// VolumeProviderIDs defines the provider ID value to set for each volume.
	VolumeProviderIDs map[VolumeUUID]string

	// FilesystemAttachmentProviderIDs defines the provider ID value to set for
	// each filesystem attachment.
	FilesystemAttachmentProviderIDs map[FilesystemAttachmentUUID]string

	// VolumeAttachmentProviderIDs defines the provider ID value to set for
	// each volume attachment.
	VolumeAttachmentProviderIDs map[VolumeAttachmentUUID]string
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
