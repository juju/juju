// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdcontext "context"
	"fmt"
	"path"
	"sync"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/schema"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/errorutils"
	"github.com/juju/juju/storage"
)

const (
	azureStorageProviderType = "azure"

	accountTypeAttr        = "account-type"
	accountTypeStandardLRS = "Standard_LRS"
	accountTypePremiumLRS  = "Premium_LRS"

	// volumeSizeMaxGiB is the maximum disk size (in gibibytes) for Azure disks.
	//
	// See: https://azure.microsoft.com/en-gb/documentation/articles/virtual-machines-disks-vhds/
	volumeSizeMaxGiB = 1023
)

// StorageProviderTypes implements storage.ProviderRegistry.
func (env *azureEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{azureStorageProviderType}, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (env *azureEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t == azureStorageProviderType {
		return &azureStorageProvider{env}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

// azureStorageProvider is a storage provider for Azure disks.
type azureStorageProvider struct {
	env *azureEnviron
}

var _ storage.Provider = (*azureStorageProvider)(nil)

var azureStorageConfigFields = schema.Fields{
	accountTypeAttr: schema.OneOf(
		schema.Const(accountTypeStandardLRS),
		schema.Const(accountTypePremiumLRS),
	),
}

var azureStorageConfigChecker = schema.FieldMap(
	azureStorageConfigFields,
	schema.Defaults{
		accountTypeAttr: accountTypeStandardLRS,
	},
)

type azureStorageConfig struct {
	storageType compute.DiskStorageAccountTypes
}

func newAzureStorageConfig(attrs map[string]interface{}) (*azureStorageConfig, error) {
	coerced, err := azureStorageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating Azure storage config")
	}
	attrs = coerced.(map[string]interface{})
	azureStorageConfig := &azureStorageConfig{
		storageType: compute.DiskStorageAccountTypes(attrs[accountTypeAttr].(string)),
	}
	return azureStorageConfig, nil
}

// ValidateConfig is part of the Provider interface.
func (e *azureStorageProvider) ValidateConfig(cfg *storage.Config) error {
	_, err := newAzureStorageConfig(cfg.Attrs())
	return errors.Trace(err)
}

// Supports is part of the Provider interface.
func (e *azureStorageProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindBlock
}

// Scope is part of the Provider interface.
func (e *azureStorageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic is part of the Provider interface.
func (e *azureStorageProvider) Dynamic() bool {
	return true
}

// Releasable is part of the Provider interface.
func (e *azureStorageProvider) Releasable() bool {
	// NOTE(axw) Azure storage is currently tied to a model, and cannot
	// be released or imported. To support releasing and importing, we'll
	// need Azure to support moving managed disks between resource groups.
	return false
}

// DefaultPools is part of the Provider interface.
func (e *azureStorageProvider) DefaultPools() []*storage.Config {
	premiumPool, _ := storage.NewConfig("azure-premium", azureStorageProviderType, map[string]interface{}{
		accountTypeAttr: accountTypePremiumLRS,
	})
	return []*storage.Config{premiumPool}
}

// VolumeSource is part of the Provider interface.
func (e *azureStorageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	return &azureVolumeSource{e.env}, nil
}

// FilesystemSource is part of the Provider interface.
func (e *azureStorageProvider) FilesystemSource(providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type azureVolumeSource struct {
	env *azureEnviron
}

// CreateVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) CreateVolumes(ctx context.ProviderCallContext, params []storage.VolumeParams) (_ []storage.CreateVolumesResult, err error) {
	results := make([]storage.CreateVolumesResult, len(params))
	for i, p := range params {
		if err := v.ValidateVolumeParams(p); err != nil {
			results[i].Error = err
			continue
		}
	}
	v.createManagedDiskVolumes(ctx, params, results)
	return results, nil
}

// createManagedDiskVolumes creates volumes with associated managed disks.
func (v *azureVolumeSource) createManagedDiskVolumes(ctx context.ProviderCallContext, params []storage.VolumeParams, results []storage.CreateVolumesResult) {
	for i, p := range params {
		if results[i].Error != nil {
			continue
		}
		volume, err := v.createManagedDiskVolume(ctx, p)
		if err != nil {
			results[i].Error = err
			continue
		}
		results[i].Volume = volume
	}
}

// createManagedDiskVolume creates a managed disk.
func (v *azureVolumeSource) createManagedDiskVolume(ctx context.ProviderCallContext, p storage.VolumeParams) (*storage.Volume, error) {
	cfg, err := newAzureStorageConfig(p.Attributes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	diskTags := make(map[string]*string)
	for k, v := range p.ResourceTags {
		diskTags[k] = to.StringPtr(v)
	}
	diskName := p.Tag.String()
	sizeInGib := mibToGib(p.Size)
	diskModel := compute.Disk{
		Name:     to.StringPtr(diskName),
		Location: to.StringPtr(v.env.location),
		Tags:     diskTags,
		Sku: &compute.DiskSku{
			Name: cfg.storageType,
		},
		DiskProperties: &compute.DiskProperties{
			CreationData: &compute.CreationData{CreateOption: compute.Empty},
			DiskSizeGB:   to.Int32Ptr(int32(sizeInGib)),
		},
	}

	diskClient := compute.DisksClient{v.env.disk}
	sdkCtx := stdcontext.Background()
	future, err := diskClient.CreateOrUpdate(sdkCtx, v.env.resourceGroup, diskName, diskModel)
	if err != nil {
		return nil, errorutils.HandleCredentialError(errors.Annotatef(err, "creating disk for volume %q", p.Tag.Id()), ctx)
	}
	err = future.WaitForCompletionRef(sdkCtx, diskClient.Client)
	if err != nil {
		return nil, errorutils.HandleCredentialError(errors.Annotatef(err, "creating disk for volume %q", p.Tag.Id()), ctx)
	}
	result, err := future.Result(diskClient)
	if err != nil && !isNotFoundResult(result.Response) {
		return nil, errors.Annotatef(err, "creating disk for volume %q", p.Tag.Id())
	}

	volume := storage.Volume{
		p.Tag,
		storage.VolumeInfo{
			VolumeId:   diskName,
			Size:       gibToMib(uint64(to.Int32(result.DiskSizeGB))),
			Persistent: true,
		},
	}
	return &volume, nil
}

// ListVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) ListVolumes(ctx context.ProviderCallContext) ([]string, error) {
	return v.listManagedDiskVolumes(ctx)
}

func (v *azureVolumeSource) listManagedDiskVolumes(ctx context.ProviderCallContext) ([]string, error) {
	var volumeIds []string
	diskClient := compute.DisksClient{v.env.disk}
	sdkCtx := stdcontext.Background()
	list, err := diskClient.ListComplete(sdkCtx)
	if err != nil {
		return nil, errorutils.HandleCredentialError(errors.Annotate(err, "listing disks"), ctx)
	}
	for ; list.NotDone(); err = list.NextWithContext(sdkCtx) {
		if err != nil {
			return nil, errorutils.HandleCredentialError(errors.Annotate(err, "listing disks"), ctx)
		}
		diskName := to.String(list.Value().Name)
		if _, err := names.ParseVolumeTag(diskName); err != nil {
			continue
		}
		volumeIds = append(volumeIds, diskName)
	}
	return volumeIds, nil
}

// DescribeVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) DescribeVolumes(ctx context.ProviderCallContext, volumeIds []string) ([]storage.DescribeVolumesResult, error) {
	return v.describeManagedDiskVolumes(ctx, volumeIds)
}

func (v *azureVolumeSource) describeManagedDiskVolumes(ctx context.ProviderCallContext, volumeIds []string) ([]storage.DescribeVolumesResult, error) {
	diskClient := compute.DisksClient{v.env.disk}
	results := make([]storage.DescribeVolumesResult, len(volumeIds))
	var wg sync.WaitGroup
	sdkCtx := stdcontext.Background()
	for i, volumeId := range volumeIds {
		wg.Add(1)
		go func(i int, volumeId string) {
			defer wg.Done()
			disk, err := diskClient.Get(sdkCtx, v.env.resourceGroup, volumeId)
			if err != nil {
				if isNotFoundResult(disk.Response) {
					err = errors.NotFoundf("disk %s", volumeId)
				}
				results[i].Error = errorutils.HandleCredentialError(err, ctx)
				return
			}
			results[i].VolumeInfo = &storage.VolumeInfo{
				VolumeId:   volumeId,
				Size:       gibToMib(uint64(to.Int32(disk.DiskSizeGB))),
				Persistent: true,
			}
		}(i, volumeId)
	}
	wg.Wait()
	return results, nil
}

// DestroyVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) DestroyVolumes(ctx context.ProviderCallContext, volumeIds []string) ([]error, error) {
	return v.destroyManagedDiskVolumes(ctx, volumeIds)
}

func (v *azureVolumeSource) destroyManagedDiskVolumes(ctx context.ProviderCallContext, volumeIds []string) ([]error, error) {
	diskClient := compute.DisksClient{v.env.disk}
	sdkCtx := stdcontext.Background()
	return foreachVolume(volumeIds, func(volumeId string) error {
		future, err := diskClient.Delete(sdkCtx, v.env.resourceGroup, volumeId)
		if err != nil {
			if !isNotFoundResponse(future.Response()) {
				return errorutils.HandleCredentialError(errors.Annotatef(err, "deleting disk %q", volumeId), ctx)
			}
			return nil
		}
		err = future.WaitForCompletionRef(sdkCtx, diskClient.Client)
		if err != nil {
			return errors.Annotatef(err, "deleting disk %q", volumeId)
		}
		result, err := future.Result(diskClient)
		if err != nil && !isNotFoundResult(result) {
			return errors.Annotatef(err, "deleting disk %q", volumeId)
		}
		return nil
	}), nil
}

func foreachVolume(volumeIds []string, f func(string) error) []error {
	results := make([]error, len(volumeIds))
	var wg sync.WaitGroup
	for i, volumeId := range volumeIds {
		wg.Add(1)
		go func(i int, volumeId string) {
			defer wg.Done()
			results[i] = f(volumeId)
		}(i, volumeId)
	}
	wg.Wait()
	return results
}

// ReleaseVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) ReleaseVolumes(ctx context.ProviderCallContext, volumeIds []string) ([]error, error) {
	// Releasing volumes is not supported, see azureStorageProvider.Releasable.
	//
	// When managed disks can be moved between resource groups, we may want to
	// support releasing unmanaged disks. We'll need to create a managed disk
	// from the blob, and then release that.
	return nil, errors.NotSupportedf("ReleaseVolumes")
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
func (v *azureVolumeSource) AttachVolumes(ctx context.ProviderCallContext, attachParams []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	results := make([]storage.AttachVolumesResult, len(attachParams))
	instanceIds := make([]instance.Id, len(attachParams))
	for i, p := range attachParams {
		instanceIds[i] = p.InstanceId
	}
	if len(instanceIds) == 0 {
		return results, nil
	}
	virtualMachines, err := v.virtualMachines(ctx, instanceIds)
	if err != nil {
		return nil, errors.Annotate(err, "getting virtual machines")
	}

	// Update VirtualMachine objects in-memory,
	// and then perform the updates all at once.
	//
	// An attachment does not require an update
	// if it is pre-existing, so we keep a record
	// of which VMs need updating.
	changed := make(map[instance.Id]bool, len(virtualMachines))
	for i, p := range attachParams {
		vm, ok := virtualMachines[p.InstanceId]
		if !ok {
			continue
		}
		if vm.err != nil {
			results[i].Error = vm.err
			continue
		}
		volumeAttachment, updated, err := v.attachVolume(vm.vm, p)
		if err != nil {
			results[i].Error = err
			vm.err = err
			continue
		}
		results[i].VolumeAttachment = volumeAttachment
		if updated {
			changed[p.InstanceId] = true
		}
	}
	for _, instanceId := range instanceIds {
		if !changed[instanceId] {
			delete(virtualMachines, instanceId)
		}
	}

	updateResults, err := v.updateVirtualMachines(ctx, virtualMachines, instanceIds)
	if err != nil {
		return nil, errors.Annotate(err, "updating virtual machines")
	}
	for i, err := range updateResults {
		if results[i].Error != nil || err == nil {
			continue
		}
		results[i].Error = err
		results[i].VolumeAttachment = nil
	}
	return results, nil
}

const azureDiskDeviceLink = "/dev/disk/azure/scsi1/lun%d"

func (v *azureVolumeSource) attachVolume(
	vm *compute.VirtualMachine,
	p storage.VolumeAttachmentParams,
) (_ *storage.VolumeAttachment, updated bool, _ error) {

	var dataDisks []compute.DataDisk
	if vm.StorageProfile.DataDisks != nil {
		dataDisks = *vm.StorageProfile.DataDisks
	}

	diskName := p.VolumeId
	for _, disk := range dataDisks {
		if to.String(disk.Name) != diskName {
			continue
		}
		// Disk is already attached.
		volumeAttachment := &storage.VolumeAttachment{
			Volume:  p.Volume,
			Machine: p.Machine,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				DeviceLink: fmt.Sprintf(azureDiskDeviceLink, to.Int32(disk.Lun)),
			},
		}
		return volumeAttachment, false, nil
	}

	volumeAttachment, err := v.addDataDisk(vm, diskName, p.Volume, p.Machine, compute.DiskCreateOptionTypesAttach, nil)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return volumeAttachment, true, nil
}

func (v *azureVolumeSource) addDataDisk(
	vm *compute.VirtualMachine,
	diskName string,
	volumeTag names.VolumeTag,
	machineTag names.Tag,
	createOption compute.DiskCreateOptionTypes,
	diskSizeGB *int32,
) (*storage.VolumeAttachment, error) {

	lun, err := nextAvailableLUN(vm)
	if err != nil {
		return nil, errors.Annotate(err, "choosing LUN")
	}

	dataDisk := compute.DataDisk{
		Lun:          to.Int32Ptr(lun),
		Name:         to.StringPtr(diskName),
		Caching:      compute.CachingTypesReadWrite,
		CreateOption: createOption,
		DiskSizeGB:   diskSizeGB,
	}
	diskResourceID := v.diskResourceID(diskName)
	dataDisk.ManagedDisk = &compute.ManagedDiskParameters{
		ID: to.StringPtr(diskResourceID),
	}

	var dataDisks []compute.DataDisk
	if vm.StorageProfile.DataDisks != nil {
		dataDisks = *vm.StorageProfile.DataDisks
	}
	dataDisks = append(dataDisks, dataDisk)
	vm.StorageProfile.DataDisks = &dataDisks

	return &storage.VolumeAttachment{
		Volume:  volumeTag,
		Machine: machineTag,
		VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
			DeviceLink: fmt.Sprintf(azureDiskDeviceLink, lun),
		},
	}, nil
}

// DetachVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) DetachVolumes(ctx context.ProviderCallContext, attachParams []storage.VolumeAttachmentParams) ([]error, error) {
	results := make([]error, len(attachParams))
	instanceIds := make([]instance.Id, len(attachParams))
	for i, p := range attachParams {
		instanceIds[i] = p.InstanceId
	}
	if len(instanceIds) == 0 {
		return results, nil
	}
	virtualMachines, err := v.virtualMachines(ctx, instanceIds)
	if err != nil {
		return nil, errors.Annotate(err, "getting virtual machines")
	}

	// Update VirtualMachine objects in-memory,
	// and then perform the updates all at once.
	//
	// An detachment does not require an update
	// if the disk isn't attached, so we keep a
	// record of which VMs need updating.
	changed := make(map[instance.Id]bool, len(virtualMachines))
	for i, p := range attachParams {
		vm, ok := virtualMachines[p.InstanceId]
		if !ok {
			continue
		}
		if vm.err != nil {
			results[i] = vm.err
			continue
		}
		if v.detachVolume(vm.vm, p) {
			changed[p.InstanceId] = true
		}
	}
	for _, instanceId := range instanceIds {
		if !changed[instanceId] {
			delete(virtualMachines, instanceId)
		}
	}

	updateResults, err := v.updateVirtualMachines(ctx, virtualMachines, instanceIds)
	if err != nil {
		return nil, errors.Annotate(err, "updating virtual machines")
	}
	for i, err := range updateResults {
		if results[i] != nil || err == nil {
			continue
		}
		results[i] = err
	}
	return results, nil
}

func (v *azureVolumeSource) detachVolume(
	vm *compute.VirtualMachine,
	p storage.VolumeAttachmentParams,
) (updated bool) {

	var dataDisks []compute.DataDisk
	if vm.StorageProfile.DataDisks != nil {
		dataDisks = *vm.StorageProfile.DataDisks
	}
	for i, disk := range dataDisks {
		if to.String(disk.Name) != p.VolumeId {
			continue
		}
		dataDisks = append(dataDisks[:i], dataDisks[i+1:]...)
		vm.StorageProfile.DataDisks = &dataDisks
		return true
	}
	return false
}

// diskResourceID returns the full resource ID for a disk, given its name.
func (v *azureVolumeSource) diskResourceID(name string) string {
	return path.Join(
		"/subscriptions",
		v.env.subscriptionId,
		"resourceGroups",
		v.env.resourceGroup,
		"providers",
		"Microsoft.Compute",
		"disks",
		name,
	)
}

type maybeVirtualMachine struct {
	vm  *compute.VirtualMachine
	err error
}

// virtualMachines returns a mapping of instance IDs to VirtualMachines and
// errors, for each of the specified instance IDs.
func (v *azureVolumeSource) virtualMachines(ctx context.ProviderCallContext, instanceIds []instance.Id) (map[instance.Id]*maybeVirtualMachine, error) {
	vmsClient := compute.VirtualMachinesClient{v.env.compute}
	sdkCtx := stdcontext.Background()
	result, err := vmsClient.ListComplete(sdkCtx, v.env.resourceGroup)
	if err != nil {
		return nil, errorutils.HandleCredentialError(errors.Annotate(err, "listing virtual machines"), ctx)
	}

	all := make(map[instance.Id]*compute.VirtualMachine)
	for ; result.NotDone(); err = result.NextWithContext(sdkCtx) {
		if err != nil {
			return nil, errors.Annotate(err, "listing disks")
		}
		vmCopy := result.Value()
		all[instance.Id(to.String(vmCopy.Name))] = &vmCopy
	}
	results := make(map[instance.Id]*maybeVirtualMachine)
	for _, id := range instanceIds {
		result := &maybeVirtualMachine{vm: all[id]}
		if result.vm == nil {
			result.err = errors.NotFoundf("instance %v", id)
		}
		results[id] = result
	}
	return results, nil
}

// updateVirtualMachines updates virtual machines in the given map by iterating
// through the list of instance IDs in order, and updating each corresponding
// virtual machine at most once.
func (v *azureVolumeSource) updateVirtualMachines(
	ctx context.ProviderCallContext,
	virtualMachines map[instance.Id]*maybeVirtualMachine, instanceIds []instance.Id,
) ([]error, error) {
	results := make([]error, len(instanceIds))
	vmsClient := compute.VirtualMachinesClient{v.env.compute}
	for i, instanceId := range instanceIds {
		vm, ok := virtualMachines[instanceId]
		if !ok {
			continue
		}
		if vm.err != nil {
			results[i] = vm.err
			continue
		}
		sdkCtx := stdcontext.Background()
		future, err := vmsClient.CreateOrUpdate(
			sdkCtx,
			v.env.resourceGroup, to.String(vm.vm.Name), *vm.vm,
		)
		if err != nil {
			if errorutils.MaybeInvalidateCredential(err, ctx) {
				return nil, errors.Trace(err)
			}
			results[i] = err
			vm.err = err
			continue
		}
		err = future.WaitForCompletionRef(sdkCtx, vmsClient.Client)
		if err != nil {
			results[i] = err
			vm.err = err
			continue
		}
		_, err = future.Result(vmsClient)
		if err != nil {
			results[i] = err
			vm.err = err
			continue
		}
		// successfully updated, don't update again
		delete(virtualMachines, instanceId)
	}
	return results, nil
}

func nextAvailableLUN(vm *compute.VirtualMachine) (int32, error) {
	// Pick the smallest LUN not in use. We have to choose them in order,
	// or the disks don't show up.
	var inUse [32]bool
	if vm.StorageProfile.DataDisks != nil {
		for _, disk := range *vm.StorageProfile.DataDisks {
			lun := to.Int32(disk.Lun)
			if lun < 0 || lun > 31 {
				logger.Debugf("ignore disk with invalid LUN: %+v", disk)
				continue
			}
			inUse[lun] = true
		}
	}
	for i, inUse := range inUse {
		if !inUse {
			return int32(i), nil
		}
	}
	return -1, errors.New("all LUNs are in use")
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
