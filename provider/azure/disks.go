// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"launchpad.net/gwacl"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

const (
	storageProviderType = storage.ProviderType("azure")
)

const (
	// volumeSizeMaxGiB is the maximum disk size (in gibibytes) for Azure disks.
	//
	// See: https://azure.microsoft.com/en-gb/documentation/articles/virtual-machines-disks-vhds/
	volumeSizeMaxGiB = 1023
)

// azureStorageProvider is a storage provider for Azure disks.
type azureStorageProvider struct{}

var _ storage.Provider = (*azureStorageProvider)(nil)

var azureStorageConfigFields = schema.Fields{}

var azureStorageConfigChecker = schema.FieldMap(
	azureStorageConfigFields,
	schema.Defaults{},
)

type azureStorageConfig struct {
}

func newAzureStorageConfig(attrs map[string]interface{}) (*azureStorageConfig, error) {
	_, err := azureStorageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating Azure storage config")
	}
	azureStorageConfig := &azureStorageConfig{}
	return azureStorageConfig, nil
}

// ValidateConfig is defined on the Provider interface.
func (e *azureStorageProvider) ValidateConfig(cfg *storage.Config) error {
	_, err := newAzureStorageConfig(cfg.Attrs())
	return errors.Trace(err)
}

// Supports is defined on the Provider interface.
func (e *azureStorageProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindBlock
}

// Scope is defined on the Provider interface.
func (e *azureStorageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic is defined on the Provider interface.
func (e *azureStorageProvider) Dynamic() bool {
	return true
}

// VolumeSource is defined on the Provider interface.
func (e *azureStorageProvider) VolumeSource(environConfig *config.Config, cfg *storage.Config) (storage.VolumeSource, error) {
	env, err := NewEnviron(environConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	uuid, ok := environConfig.UUID()
	if !ok {
		return nil, errors.NotFoundf("environment UUID")
	}
	source := &azureVolumeSource{
		env:     env,
		envName: environConfig.Name(),
		envUUID: uuid,
	}
	return source, nil
}

// FilesystemSource is defined on the Provider interface.
func (e *azureStorageProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type azureVolumeSource struct {
	env     *azureEnviron
	envName string // non-unique, informational only
	envUUID string
}

var _ storage.VolumeSource = (*azureVolumeSource)(nil)

// CreateVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) CreateVolumes(params []storage.VolumeParams) (_ []storage.CreateVolumesResult, err error) {

	// First, validate the params before we use them.
	results := make([]storage.CreateVolumesResult, len(params))
	for i, p := range params {
		if err := v.ValidateVolumeParams(p); err != nil {
			results[i].Error = err
			continue
		}
	}

	// TODO(axw) cache results of GetRole from createVolume for multiple
	// attachments to the same machine. When doing so, be careful to
	// ensure the cached role's in-use LUNs are updated between attachments.

	for i, p := range params {
		if results[i].Error != nil {
			continue
		}
		volume, volumeAttachment, err := v.createVolume(p)
		if err != nil {
			results[i].Error = err
			continue
		}
		results[i].Volume = volume
		results[i].VolumeAttachment = volumeAttachment
	}
	return results, nil
}

func (v *azureVolumeSource) createVolume(p storage.VolumeParams) (*storage.Volume, *storage.VolumeAttachment, error) {
	cloudServiceName, roleName := v.env.splitInstanceId(p.Attachment.InstanceId)
	if roleName == "" {
		return nil, nil, errors.NotSupportedf("attaching disks to legacy instances")
	}
	deploymentName := deploymentNameV2(cloudServiceName)

	// Get the role first so we know which LUNs are in use.
	role, err := v.getRole(p.Attachment.InstanceId)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting role")
	}
	lun, err := nextAvailableLUN(role)
	if err != nil {
		return nil, nil, errors.Annotate(err, "choosing LUN")
	}

	diskLabel := fmt.Sprintf("%s%s", v.env.getEnvPrefix(), p.Tag.String())
	vhdFilename := p.Tag.String() + ".vhd"
	mediaLink := v.vhdMediaLinkPrefix() + vhdFilename

	// Create and attach a disk to the instance.
	sizeInGib := mibToGib(p.Size)
	if err := v.env.api.AddDataDisk(&gwacl.AddDataDiskRequest{
		ServiceName:    cloudServiceName,
		DeploymentName: deploymentName,
		RoleName:       roleName,
		DataVirtualHardDisk: gwacl.DataVirtualHardDisk{
			DiskLabel:           diskLabel,
			LogicalDiskSizeInGB: int(sizeInGib),
			MediaLink:           mediaLink,
			LUN:                 lun,
		},
	}); err != nil {
		return nil, nil, errors.Annotate(err, "adding data disk")
	}

	// Data disks associate VHDs to machines. In Juju's storage model,
	// the VHD is the volume and the disk is the volume attachment.
	//
	// Note that we don't currently support attaching/detaching volumes
	// (see note on Persistent below), but using the VHD name as the
	// volume ID at least allows that as a future option.
	volumeId := vhdFilename

	volume := storage.Volume{
		p.Tag,
		storage.VolumeInfo{
			VolumeId: volumeId,
			Size:     gibToMib(sizeInGib),
			// We don't currently support persistent volumes in
			// Azure, as it requires removal of "comp=media" when
			// deleting VMs, complicating cleanup.
			Persistent: false,
		},
	}
	volumeAttachment := storage.VolumeAttachment{
		p.Tag,
		p.Attachment.Machine,
		storage.VolumeAttachmentInfo{
			BusAddress: diskBusAddress(lun),
		},
	}
	return &volume, &volumeAttachment, nil
}

// vhdMediaLinkPrefix returns the media link prefix for disks
// associated with the environment. gwacl's helper returns
// http scheme URLs; we use https to simplify matching what
// Azure returns. Azure always returns https, even if you
// specified http originally.
func (v *azureVolumeSource) vhdMediaLinkPrefix() string {
	storageAccount := v.env.ecfg.storageAccountName()
	dir := path.Join("vhds", v.envUUID)
	mediaLink := gwacl.CreateVirtualHardDiskMediaLink(storageAccount, dir) + "/"
	mediaLink = "https://" + mediaLink[len("http://"):]
	return mediaLink
}

// ListVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) ListVolumes() ([]string, error) {
	disks, err := v.listDisks()
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumeIds := make([]string, len(disks))
	for i, disk := range disks {
		_, volumeId := path.Split(disk.MediaLink)
		volumeIds[i] = volumeId
	}
	return volumeIds, nil
}

func (v *azureVolumeSource) listDisks() ([]gwacl.Disk, error) {
	disks, err := v.env.api.ListDisks()
	if err != nil {
		return nil, errors.Annotate(err, "listing disks")
	}
	mediaLinkPrefix := v.vhdMediaLinkPrefix()
	matching := make([]gwacl.Disk, 0, len(disks))
	for _, disk := range disks {
		if strings.HasPrefix(disk.MediaLink, mediaLinkPrefix) {
			matching = append(matching, disk)
		}
	}
	return matching, nil
}

// DescribeVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) DescribeVolumes(volIds []string) ([]storage.DescribeVolumesResult, error) {
	disks, err := v.listDisks()
	if err != nil {
		return nil, errors.Annotate(err, "listing disks")
	}

	byVolumeId := make(map[string]gwacl.Disk)
	for _, disk := range disks {
		_, volumeId := path.Split(disk.MediaLink)
		byVolumeId[volumeId] = disk
	}

	results := make([]storage.DescribeVolumesResult, len(volIds))
	for i, volumeId := range volIds {
		disk, ok := byVolumeId[volumeId]
		if !ok {
			results[i].Error = errors.NotFoundf("volume %v", volumeId)
			continue
		}
		results[i].VolumeInfo = &storage.VolumeInfo{
			VolumeId: volumeId,
			Size:     gibToMib(uint64(disk.LogicalSizeInGB)),
			// We don't support persistent volumes at the moment;
			// see CreateVolumes.
			Persistent: false,
		}
	}

	return results, nil
}

// DestroyVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) DestroyVolumes(volIds []string) ([]error, error) {
	// We don't currently support persistent volumes.
	return nil, errors.NotSupportedf("DestroyVolumes")
}

// ValidateVolumeParams is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	if mibToGib(params.Size) > volumeSizeMaxGiB {
		return errors.Errorf(
			"%d GiB exceeds the maximum of %d GiB",
			mibToGib(params.Size),
			volumeSizeMaxGiB,
		)
	}
	return nil
}

// AttachVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) AttachVolumes(attachParams []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	// We don't currently support persistent volumes, but we do need to
	// support "reattaching" volumes to machines; i.e. just verify that
	// the attachment is in place, and fail otherwise.

	type maybeRole struct {
		role *gwacl.PersistentVMRole
		err  error
	}

	roles := make(map[instance.Id]maybeRole)
	for _, p := range attachParams {
		if _, ok := roles[p.InstanceId]; ok {
			continue
		}
		role, err := v.getRole(p.InstanceId)
		roles[p.InstanceId] = maybeRole{role, err}
	}

	results := make([]storage.AttachVolumesResult, len(attachParams))
	for i, p := range attachParams {
		maybeRole := roles[p.InstanceId]
		if maybeRole.err != nil {
			results[i].Error = maybeRole.err
			continue
		}
		volumeAttachment, err := v.attachVolume(p, maybeRole.role)
		if err != nil {
			results[i].Error = err
			continue
		}
		results[i].VolumeAttachment = volumeAttachment
	}
	return results, nil
}

func (v *azureVolumeSource) attachVolume(
	p storage.VolumeAttachmentParams,
	role *gwacl.PersistentVMRole,
) (*storage.VolumeAttachment, error) {

	var disks []gwacl.DataVirtualHardDisk
	if role.DataVirtualHardDisks != nil {
		disks = *role.DataVirtualHardDisks
	}

	// Check if the disk is already attached to the machine.
	mediaLinkPrefix := v.vhdMediaLinkPrefix()
	for _, disk := range disks {
		if !strings.HasPrefix(disk.MediaLink, mediaLinkPrefix) {
			continue
		}
		_, volumeId := path.Split(disk.MediaLink)
		if volumeId != p.VolumeId {
			continue
		}
		return &storage.VolumeAttachment{
			p.Volume,
			p.Machine,
			storage.VolumeAttachmentInfo{
				BusAddress: diskBusAddress(disk.LUN),
			},
		}, nil
	}

	// If the disk is not attached already, the AttachVolumes call must
	// fail. We do not support persistent volumes at the moment, and if
	// we get here it means that the disk has been detached out of band.
	return nil, errors.NotSupportedf("attaching volumes")
}

// DetachVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) DetachVolumes(attachParams []storage.VolumeAttachmentParams) ([]error, error) {
	// We don't currently support persistent volumes.
	return nil, errors.NotSupportedf("detaching volumes")
}

func (v *azureVolumeSource) getRole(id instance.Id) (*gwacl.PersistentVMRole, error) {
	cloudServiceName, roleName := v.env.splitInstanceId(id)
	if roleName == "" {
		return nil, errors.NotSupportedf("attaching disks to legacy instances")
	}
	deploymentName := deploymentNameV2(cloudServiceName)
	return v.env.api.GetRole(&gwacl.GetRoleRequest{
		ServiceName:    cloudServiceName,
		DeploymentName: deploymentName,
		RoleName:       roleName,
	})
}

func nextAvailableLUN(role *gwacl.PersistentVMRole) (int, error) {
	// Pick the smallest LUN not in use. We have to choose them in order,
	// or the disks don't show up.
	var inUse [32]bool
	if role.DataVirtualHardDisks != nil {
		for _, disk := range *role.DataVirtualHardDisks {
			if disk.LUN < 0 || disk.LUN > 31 {
				logger.Warningf("ignore disk with invalid LUN: %+v", disk)
				continue
			}
			inUse[disk.LUN] = true
		}
	}
	for i, inUse := range inUse {
		if !inUse {
			return i, nil
		}
	}
	return -1, errors.New("all LUNs are in use")
}

// diskBusAddress returns the value to use in the BusAddress field of
// VolumeAttachmentInfo for a disk with the specified LUN.
func diskBusAddress(lun int) string {
	return fmt.Sprintf("scsi@5:0.0.%d", lun)
}

// mibToGib converts mebibytes to gibibytes.
// AWS expects GiB, we work in MiB; round up
// to nearest GiB.
func mibToGib(m uint64) uint64 {
	return (m + 1023) / 1024
}

// gibToMib converts gibibytes to mebibytes.
func gibToMib(g uint64) uint64 {
	return g * 1024
}
