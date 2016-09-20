// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	armstorage "github.com/Azure/azure-sdk-for-go/arm/storage"
	azurestorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/azure/internal/armtemplates"
	internalazurestorage "github.com/juju/juju/provider/azure/internal/azurestorage"
	"github.com/juju/juju/storage"
)

const (
	azureStorageProviderType = "azure"

	// volumeSizeMaxGiB is the maximum disk size (in gibibytes) for Azure disks.
	//
	// See: https://azure.microsoft.com/en-gb/documentation/articles/virtual-machines-disks-vhds/
	volumeSizeMaxGiB = 1023

	// osDiskVHDContainer is the name of the blob container for VHDs
	// backing OS disks.
	osDiskVHDContainer = "osvhds"

	// dataDiskVHDContainer is the name of the blob container for VHDs
	// backing data disks.
	dataDiskVHDContainer = "datavhds"

	// vhdExtension is the filename extension we give to VHDs we create.
	vhdExtension = ".vhd"
)

// StorageProviderTypes implements storage.ProviderRegistry.
func (env *azureEnviron) StorageProviderTypes() []storage.ProviderType {
	return []storage.ProviderType{azureStorageProviderType}
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

// DefaultPools is part of the Provider interface.
func (e *azureStorageProvider) DefaultPools() []*storage.Config {
	return nil
}

// VolumeSource is part of the Provider interface.
func (e *azureStorageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	if err := e.ValidateConfig(cfg); err != nil {
		return nil, errors.Trace(err)
	}
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
func (v *azureVolumeSource) CreateVolumes(params []storage.VolumeParams) (_ []storage.CreateVolumesResult, err error) {

	// First, validate the params before we use them.
	results := make([]storage.CreateVolumesResult, len(params))
	var instanceIds []instance.Id
	for i, p := range params {
		if err := v.ValidateVolumeParams(p); err != nil {
			results[i].Error = err
			continue
		}
		instanceIds = append(instanceIds, p.Attachment.InstanceId)
	}
	if len(instanceIds) == 0 {
		return results, nil
	}
	virtualMachines, err := v.virtualMachines(instanceIds)
	if err != nil {
		return nil, errors.Annotate(err, "getting virtual machines")
	}
	storageAccount, err := v.env.getStorageAccount(false)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Update VirtualMachine objects in-memory,
	// and then perform the updates all at once.
	for i, p := range params {
		if results[i].Error != nil {
			continue
		}
		vm, ok := virtualMachines[p.Attachment.InstanceId]
		if !ok {
			continue
		}
		if vm.err != nil {
			results[i].Error = vm.err
			continue
		}
		volume, volumeAttachment, err := v.createVolume(
			vm.vm, p, storageAccount,
		)
		if err != nil {
			results[i].Error = err
			vm.err = err
			continue
		}
		results[i].Volume = volume
		results[i].VolumeAttachment = volumeAttachment
	}

	updateResults, err := v.updateVirtualMachines(virtualMachines, instanceIds)
	if err != nil {
		return nil, errors.Annotate(err, "updating virtual machines")
	}
	for i, err := range updateResults {
		if results[i].Error != nil || err == nil {
			continue
		}
		results[i].Error = err
		results[i].Volume = nil
		results[i].VolumeAttachment = nil
	}
	return results, nil
}

// createVolume updates the provided VirtualMachine's StorageProfile with the
// parameters for creating a new data disk. We don't actually interact with
// the Azure API until after all changes to the VirtualMachine are made.
func (v *azureVolumeSource) createVolume(
	vm *compute.VirtualMachine,
	p storage.VolumeParams,
	storageAccount *armstorage.Account,
) (*storage.Volume, *storage.VolumeAttachment, error) {

	lun, err := nextAvailableLUN(vm)
	if err != nil {
		return nil, nil, errors.Annotate(err, "choosing LUN")
	}

	dataDisksRoot := dataDiskVhdRoot(storageAccount)
	dataDiskName := p.Tag.String()
	vhdURI := dataDisksRoot + dataDiskName + vhdExtension

	sizeInGib := mibToGib(p.Size)
	dataDisk := compute.DataDisk{
		Lun:          to.Int32Ptr(lun),
		DiskSizeGB:   to.Int32Ptr(int32(sizeInGib)),
		Name:         to.StringPtr(dataDiskName),
		Vhd:          &compute.VirtualHardDisk{to.StringPtr(vhdURI)},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Empty,
	}

	var dataDisks []compute.DataDisk
	if vm.Properties.StorageProfile.DataDisks != nil {
		dataDisks = *vm.Properties.StorageProfile.DataDisks
	}
	dataDisks = append(dataDisks, dataDisk)
	vm.Properties.StorageProfile.DataDisks = &dataDisks

	// Data disks associate VHDs to machines. In Juju's storage model,
	// the VHD is the volume and the disk is the volume attachment.
	volume := storage.Volume{
		p.Tag,
		storage.VolumeInfo{
			VolumeId: dataDiskName,
			Size:     gibToMib(sizeInGib),
			// We don't currently support persistent volumes in
			// Azure, as it requires removal of "comp=media" when
			// deleting VMs, complicating cleanup.
			Persistent: true,
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

// ListVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) ListVolumes() ([]string, error) {
	blobs, err := v.listBlobs()
	if err != nil {
		return nil, errors.Annotate(err, "listing volumes")
	}
	volumeIds := make([]string, 0, len(blobs))
	for _, blob := range blobs {
		volumeId, ok := blobVolumeId(blob)
		if !ok {
			continue
		}
		volumeIds = append(volumeIds, volumeId)
	}
	return volumeIds, nil
}

// listBlobs returns a list of blobs in the data-disk container.
func (v *azureVolumeSource) listBlobs() ([]azurestorage.Blob, error) {
	client, err := v.env.getStorageClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	blobsClient := client.GetBlobService()
	// TODO(axw) handle pagination
	// TODO(axw) consider taking a set of IDs and computing the
	//           longest common prefix to pass in the parameters
	response, err := blobsClient.ListBlobs(
		dataDiskVHDContainer, azurestorage.ListBlobsParameters{},
	)
	if err != nil {
		if err, ok := err.(azurestorage.AzureStorageServiceError); ok {
			switch err.Code {
			case "ContainerNotFound":
				return nil, nil
			}
		}
		return nil, errors.Annotate(err, "listing blobs")
	}
	return response.Blobs, nil
}

// DescribeVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) DescribeVolumes(volumeIds []string) ([]storage.DescribeVolumesResult, error) {
	blobs, err := v.listBlobs()
	if err != nil {
		return nil, errors.Annotate(err, "listing volumes")
	}

	byVolumeId := make(map[string]azurestorage.Blob)
	for _, blob := range blobs {
		volumeId, ok := blobVolumeId(blob)
		if !ok {
			continue
		}
		byVolumeId[volumeId] = blob
	}

	results := make([]storage.DescribeVolumesResult, len(volumeIds))
	for i, volumeId := range volumeIds {
		blob, ok := byVolumeId[volumeId]
		if !ok {
			results[i].Error = errors.NotFoundf("%s", volumeId)
			continue
		}
		sizeInMib := blob.Properties.ContentLength / (1024 * 1024)
		results[i].VolumeInfo = &storage.VolumeInfo{
			VolumeId:   volumeId,
			Size:       uint64(sizeInMib),
			Persistent: true,
		}
	}

	return results, nil
}

// DestroyVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) DestroyVolumes(volumeIds []string) ([]error, error) {
	client, err := v.env.getStorageClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	blobsClient := client.GetBlobService()
	results := make([]error, len(volumeIds))
	for i, volumeId := range volumeIds {
		_, err := blobsClient.DeleteBlobIfExists(
			dataDiskVHDContainer, volumeId+vhdExtension, nil,
		)
		results[i] = err
	}
	return results, nil
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
	results := make([]storage.AttachVolumesResult, len(attachParams))
	instanceIds := make([]instance.Id, len(attachParams))
	for i, p := range attachParams {
		instanceIds[i] = p.InstanceId
	}
	if len(instanceIds) == 0 {
		return results, nil
	}
	virtualMachines, err := v.virtualMachines(instanceIds)
	if err != nil {
		return nil, errors.Annotate(err, "getting virtual machines")
	}
	storageAccount, err := v.env.getStorageAccount(false)
	if err != nil {
		return nil, errors.Trace(err)
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
		volumeAttachment, updated, err := v.attachVolume(
			vm.vm, p, storageAccount,
		)
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

	updateResults, err := v.updateVirtualMachines(virtualMachines, instanceIds)
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

func (v *azureVolumeSource) attachVolume(
	vm *compute.VirtualMachine,
	p storage.VolumeAttachmentParams,
	storageAccount *armstorage.Account,
) (_ *storage.VolumeAttachment, updated bool, _ error) {

	storageAccount, err := v.env.getStorageAccount(false)
	if err != nil {
		return nil, false, errors.Trace(err)
	}

	dataDisksRoot := dataDiskVhdRoot(storageAccount)
	dataDiskName := p.VolumeId
	vhdURI := dataDisksRoot + dataDiskName + vhdExtension

	var dataDisks []compute.DataDisk
	if vm.Properties.StorageProfile.DataDisks != nil {
		dataDisks = *vm.Properties.StorageProfile.DataDisks
	}
	for _, disk := range dataDisks {
		if to.String(disk.Name) != p.VolumeId {
			continue
		}
		if to.String(disk.Vhd.URI) != vhdURI {
			continue
		}
		// Disk is already attached.
		volumeAttachment := &storage.VolumeAttachment{
			p.Volume,
			p.Machine,
			storage.VolumeAttachmentInfo{
				BusAddress: diskBusAddress(to.Int32(disk.Lun)),
			},
		}
		return volumeAttachment, false, nil
	}

	lun, err := nextAvailableLUN(vm)
	if err != nil {
		return nil, false, errors.Annotate(err, "choosing LUN")
	}

	dataDisk := compute.DataDisk{
		Lun:          to.Int32Ptr(lun),
		Name:         to.StringPtr(dataDiskName),
		Vhd:          &compute.VirtualHardDisk{to.StringPtr(vhdURI)},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Attach,
	}
	dataDisks = append(dataDisks, dataDisk)
	vm.Properties.StorageProfile.DataDisks = &dataDisks

	volumeAttachment := storage.VolumeAttachment{
		p.Volume,
		p.Machine,
		storage.VolumeAttachmentInfo{
			BusAddress: diskBusAddress(lun),
		},
	}
	return &volumeAttachment, true, nil
}

// DetachVolumes is specified on the storage.VolumeSource interface.
func (v *azureVolumeSource) DetachVolumes(attachParams []storage.VolumeAttachmentParams) ([]error, error) {
	results := make([]error, len(attachParams))
	instanceIds := make([]instance.Id, len(attachParams))
	for i, p := range attachParams {
		instanceIds[i] = p.InstanceId
	}
	if len(instanceIds) == 0 {
		return results, nil
	}
	virtualMachines, err := v.virtualMachines(instanceIds)
	if err != nil {
		return nil, errors.Annotate(err, "getting virtual machines")
	}
	storageAccount, err := v.env.getStorageAccount(false)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage account")
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
		if v.detachVolume(vm.vm, p, storageAccount) {
			changed[p.InstanceId] = true
		}
	}
	for _, instanceId := range instanceIds {
		if !changed[instanceId] {
			delete(virtualMachines, instanceId)
		}
	}

	updateResults, err := v.updateVirtualMachines(virtualMachines, instanceIds)
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
	storageAccount *armstorage.Account,
) (updated bool) {

	dataDisksRoot := dataDiskVhdRoot(storageAccount)
	dataDiskName := p.VolumeId
	vhdURI := dataDisksRoot + dataDiskName + vhdExtension

	var dataDisks []compute.DataDisk
	if vm.Properties.StorageProfile.DataDisks != nil {
		dataDisks = *vm.Properties.StorageProfile.DataDisks
	}
	for i, disk := range dataDisks {
		if to.String(disk.Name) != p.VolumeId {
			continue
		}
		if to.String(disk.Vhd.URI) != vhdURI {
			continue
		}
		dataDisks = append(dataDisks[:i], dataDisks[i+1:]...)
		if len(dataDisks) == 0 {
			vm.Properties.StorageProfile.DataDisks = nil
		} else {
			*vm.Properties.StorageProfile.DataDisks = dataDisks
		}
		return true
	}
	return false
}

type maybeVirtualMachine struct {
	vm  *compute.VirtualMachine
	err error
}

// virtualMachines returns a mapping of instance IDs to VirtualMachines and
// errors, for each of the specified instance IDs.
func (v *azureVolumeSource) virtualMachines(instanceIds []instance.Id) (map[instance.Id]*maybeVirtualMachine, error) {
	vmsClient := compute.VirtualMachinesClient{v.env.compute}
	var result compute.VirtualMachineListResult
	if err := v.env.callAPI(func() (autorest.Response, error) {
		var err error
		result, err = vmsClient.List(v.env.resourceGroup)
		return result.Response, err
	}); err != nil {
		return nil, errors.Annotate(err, "listing virtual machines")
	}

	all := make(map[instance.Id]*compute.VirtualMachine)
	if result.Value != nil {
		for _, vm := range *result.Value {
			vmCopy := vm
			all[instance.Id(to.String(vm.Name))] = &vmCopy
		}
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
		if err := v.env.callAPI(func() (autorest.Response, error) {
			return vmsClient.CreateOrUpdate(
				v.env.resourceGroup, to.String(vm.vm.Name), *vm.vm,
				nil, // abort channel
			)
		}); err != nil {
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
	if vm.Properties.StorageProfile.DataDisks != nil {
		for _, disk := range *vm.Properties.StorageProfile.DataDisks {
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

// diskBusAddress returns the value to use in the BusAddress field of
// VolumeAttachmentInfo for a disk with the specified LUN.
func diskBusAddress(lun int32) string {
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

// osDiskVhdRoot returns the URL to the blob container in which we store the
// VHDs for OS disks for the environment.
func osDiskVhdRoot(storageAccount *armstorage.Account) string {
	return blobContainerURL(storageAccount, osDiskVHDContainer)
}

// dataDiskVhdRoot returns the URL to the blob container in which we store the
// VHDs for data disks for the environment.
func dataDiskVhdRoot(storageAccount *armstorage.Account) string {
	return blobContainerURL(storageAccount, dataDiskVHDContainer)
}

// blobContainer returns the URL to the named blob container.
func blobContainerURL(storageAccount *armstorage.Account, container string) string {
	return fmt.Sprintf(
		"%s%s/",
		to.String(storageAccount.Properties.PrimaryEndpoints.Blob),
		container,
	)
}

// blobVolumeId returns the volume ID for a blob, and a boolean reporting
// whether or not the blob's name matches the scheme we use.
func blobVolumeId(blob azurestorage.Blob) (string, bool) {
	if !strings.HasSuffix(blob.Name, vhdExtension) {
		return "", false
	}
	volumeId := blob.Name[:len(blob.Name)-len(vhdExtension)]
	if _, err := names.ParseVolumeTag(volumeId); err != nil {
		return "", false
	}
	return volumeId, true
}

// getStorageClient returns a new storage client, given an environ config
// and a constructor.
func getStorageClient(
	newClient internalazurestorage.NewClientFunc,
	storageEndpoint string,
	storageAccount *armstorage.Account,
	storageAccountKey *armstorage.AccountKey,
) (internalazurestorage.Client, error) {
	storageAccountName := to.String(storageAccount.Name)
	const useHTTPS = true
	return newClient(
		storageAccountName,
		to.String(storageAccountKey.Value),
		storageEndpoint,
		azurestorage.DefaultAPIVersion,
		useHTTPS,
	)
}

// getStorageAccountKey returns the key for the storage account.
func getStorageAccountKey(
	callAPI callAPIFunc,
	client armstorage.AccountsClient,
	resourceGroup, accountName string,
) (*armstorage.AccountKey, error) {
	logger.Debugf("getting keys for storage account %q", accountName)
	var listKeysResult armstorage.AccountListKeysResult
	if err := callAPI(func() (autorest.Response, error) {
		var err error
		listKeysResult, err = client.ListKeys(resourceGroup, accountName)
		return listKeysResult.Response, err
	}); err != nil {
		if listKeysResult.Response.Response != nil && listKeysResult.StatusCode == http.StatusNotFound {
			return nil, errors.NewNotFound(err, "storage account keys not found")
		}
		return nil, errors.Annotate(err, "listing storage account keys")
	}
	if listKeysResult.Keys == nil {
		return nil, errors.NotFoundf("storage account keys")
	}

	// We need a storage key with full permissions.
	var fullKey *armstorage.AccountKey
	for _, key := range *listKeysResult.Keys {
		logger.Debugf("storage account key: %#v", key)
		// At least some of the time, Azure returns the permissions
		// in title-case, which does not match the constant.
		if strings.ToUpper(string(key.Permissions)) != string(armstorage.FULL) {
			continue
		}
		fullKey = &key
		break
	}
	if fullKey == nil {
		return nil, errors.NotFoundf(
			"storage account key with %q permission",
			armstorage.FULL,
		)
	}
	return fullKey, nil
}

// storageAccountTemplateResource returns a template resource definition
// for creating a storage account.
func storageAccountTemplateResource(
	location string,
	envTags map[string]string,
	accountName, accountType string,
) armtemplates.Resource {
	return armtemplates.Resource{
		APIVersion: armstorage.APIVersion,
		Type:       "Microsoft.Storage/storageAccounts",
		Name:       accountName,
		Location:   location,
		Tags:       envTags,
		StorageSku: &armstorage.Sku{
			Name: armstorage.SkuName(accountType),
		},
	}
}
