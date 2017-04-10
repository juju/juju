// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/progress"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
	tomb "gopkg.in/tomb.v1"

	"github.com/juju/juju/constraints"
)

// CreateVirtualMachineParams contains the parameters required for creating
// a new virtual machine.
type CreateVirtualMachineParams struct {
	// Name is the name to give the virtual machine. The VM name is used
	// for its hostname also.
	Name string

	// Folder is the path of the VM folder, relative to the root VM folder,
	// in which to create the VM.
	Folder string

	// OVAContentsDir is the directory containing the extracted OVA contents.
	OVADir string

	// OVF contains the OVF content.
	OVF string

	// UserData is the cloud-init user-data.
	UserData string

	// ComputeResource is the compute resource (host or cluster) to be used
	// to create the VM.
	ComputeResource *mo.ComputeResource

	// Metadata are metadata key/value pairs to apply to the VM as
	// "extra config".
	Metadata map[string]string

	// Constraints contains the resource constraints for the virtual machine.
	Constraints constraints.Value

	// ExternalNetwork, if set, is the name of an additional "external"
	// network to which the VM should be connected.
	ExternalNetwork string

	// UpdateProgress is a function that should be called before/during
	// long-running operations to provide a progress reporting.
	UpdateProgress func(string)
}

// CreateVirtualMachine creates and powers on a new VM.
//
// This method imports an OVF template using the vSphere API. This process
// comprises the following steps:
//   1. Download the OVA archive, extract it, and load the OVF file contained
//      within. This must have happened before CreateVirtualMachine is called.
//   2. Call CreateImportSpec [0], which validates the OVF descriptor against
//      the hardware supported by the host system. If the validation succeeds,
//      the method returns a result containing:
//        - an ImportSpec to use for importing the entity
//        - a list of items to upload from the OVA (e.g. VMDKs)
//   3. Prepare all necessary parameters (CPU, memory, root disk, etc.), and
//      call the ImportVApp method [0]. This method is responsible for actually
//      creating the VM. An HttpNfcLease [1] object is returned, which is used
//      to signal completion of the process.
//   4. Upload virtual disk contents (usually consists of a single VMDK file)
//   5. Call HttpNfcLeaseComplete [0] to signal completion of uploading,
//      completing the process of creating the virtual machine.
//
// [0] https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/
// [1] https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/vim.HttpNfcLease.html
func (c *Client) CreateVirtualMachine(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (*mo.VirtualMachine, error) {

	args.UpdateProgress("creating import spec")
	spec, err := c.createImportSpec(ctx, args)
	if err != nil {
		return nil, errors.Annotate(err, "creating import spec")
	}

	// Locate the folder in which to create the VM.
	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	folderPath := path.Join(folders.VmFolder.InventoryPath, args.Folder)
	vmFolder, err := finder.Folder(ctx, folderPath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Import the VApp.
	args.UpdateProgress(fmt.Sprintf("creating VM %q", args.Name))
	c.logger.Debugf("creating VM in folder %s", vmFolder)
	rp := object.NewResourcePool(c.client.Client, *args.ComputeResource.ResourcePool)
	lease, err := rp.ImportVApp(ctx, spec.ImportSpec, vmFolder, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to import vapp")
	}

	// Upload the VMDK.
	info, err := lease.Wait(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	type uploadItem struct {
		item types.OvfFileItem
		url  *url.URL
	}
	var uploadItems []uploadItem
	for _, device := range info.DeviceUrl {
		for _, item := range spec.FileItem {
			if device.ImportKey != item.DeviceId {
				continue
			}
			u, err := c.client.Client.ParseURL(device.Url)
			if err != nil {
				return nil, errors.Trace(err)
			}
			uploadItems = append(uploadItems, uploadItem{
				item: item,
				url:  u,
			})
		}
	}
	for _, item := range uploadItems {
		if err := uploadImage(
			c.client.Client,
			item.item,
			args.OVADir,
			item.url,
			args.UpdateProgress,
		); err != nil {
			return nil, errors.Trace(err)
		}
	}
	if err := lease.HttpNfcLeaseComplete(ctx); err != nil {
		return nil, errors.Trace(err)
	}

	// Finally, power on and return the VM.
	args.UpdateProgress("powering on")
	vm := object.NewVirtualMachine(c.client.Client, info.Entity)
	task, err := vm.PowerOn(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	taskInfo, err := task.WaitForResult(ctx, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var res mo.VirtualMachine
	if err := c.client.RetrieveOne(ctx, *taskInfo.Entity, nil, &res); err != nil {
		return nil, errors.Trace(err)
	}
	return &res, nil
}

func (c *Client) createImportSpec(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (*types.OvfCreateImportSpecResult, error) {
	cisp := types.OvfCreateImportSpecParams{
		EntityName: args.Name,
		PropertyMapping: []types.KeyValue{
			types.KeyValue{Key: "user-data", Value: string(args.UserData)},
			types.KeyValue{Key: "hostname", Value: string(args.Name)},
		},
	}
	ovfManager := object.NewOvfManager(c.client.Client)
	resourcePool := object.NewReference(c.client.Client, *args.ComputeResource.ResourcePool)
	datastore := object.NewReference(c.client.Client, args.ComputeResource.Datastore[0])
	spec, err := ovfManager.CreateImportSpec(ctx, args.OVF, resourcePool, datastore, cisp)
	if err != nil {
		return nil, errors.Trace(err)
	} else if spec.Error != nil {
		return nil, errors.New(spec.Error[0].LocalizedMessage)
	}
	s := &spec.ImportSpec.(*types.VirtualMachineImportSpec).ConfigSpec

	// Apply resource constraints.
	if args.Constraints.HasCpuCores() {
		s.NumCPUs = int32(*args.Constraints.CpuCores)
	}
	if args.Constraints.HasMem() {
		s.MemoryMB = int64(*args.Constraints.Mem)
	}
	var cpuPower int64
	if args.Constraints.HasCpuPower() {
		cpuPower = int64(*args.Constraints.CpuPower)
	}
	s.CpuAllocation = &types.ResourceAllocationInfo{
		Limit:       cpuPower,
		Reservation: cpuPower,
	}
	for _, d := range s.DeviceChange {
		disk, ok := d.GetVirtualDeviceConfigSpec().Device.(*types.VirtualDisk)
		if !ok {
			continue
		}
		var rootDisk int64
		if args.Constraints.RootDisk != nil {
			rootDisk = int64(*args.Constraints.RootDisk) * 1024
		}
		if disk.CapacityInKB < rootDisk {
			disk.CapacityInKB = rootDisk
		}
		// Set UnitNumber to -1 if it is unset in ovf file template
		// (in this case it is parses as 0), because 0 causes an error
		// for disk devices.
		var unitNumber int32
		if disk.UnitNumber != nil {
			unitNumber = *disk.UnitNumber
		}
		if unitNumber == 0 {
			unitNumber = -1
			disk.UnitNumber = &unitNumber
		}
	}

	// Apply metadata. Note that we do not have the ability set create or
	// apply tags that will show up in vCenter, as that requires a separate
	// vSphere Automation that we do not have an SDK for.
	for k, v := range args.Metadata {
		s.ExtraConfig = append(s.ExtraConfig, &types.OptionValue{Key: k, Value: v})
	}

	// Add the external network, if any.
	if args.ExternalNetwork != "" {
		s.DeviceChange = append(s.DeviceChange, &types.VirtualDeviceConfigSpec{
			Operation: types.VirtualDeviceConfigSpecOperationAdd,
			Device: &types.VirtualE1000{
				VirtualEthernetCard: types.VirtualEthernetCard{
					VirtualDevice: types.VirtualDevice{
						Backing: &types.VirtualEthernetCardNetworkBackingInfo{
							VirtualDeviceDeviceBackingInfo: types.VirtualDeviceDeviceBackingInfo{
								DeviceName: args.ExternalNetwork,
							},
						},
						Connectable: &types.VirtualDeviceConnectInfo{
							StartConnected:    true,
							AllowGuestControl: true,
						},
					},
				},
			},
		})
	}
	return spec, nil
}

// uploadImage uploads an image from the given extracted OVA directory
// to a target URL.
func uploadImage(
	client *vim25.Client,
	item types.OvfFileItem,
	ovaDir string,
	targetURL *url.URL,
	updateProgress func(string),
) error {
	sourcePath := filepath.Join(ovaDir, item.Path)
	f, err := os.Open(sourcePath)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()

	// Transfer image upload progress to the UpdateProgress function.
	progressChan := make(chan progress.Report)
	progressSink := progressUpdater{
		ch:     progressChan,
		update: updateProgress,
		action: fmt.Sprintf("uploading %s", item.Path),
	}
	go progressSink.loop()
	defer progressSink.done()

	opts := soap.Upload{
		Method:        "POST",
		Type:          "application/x-vnd.vmware-streamVmdk",
		ContentLength: item.Size,
		Progress:      &progressSink,
	}
	if err := client.Upload(f, targetURL, &opts); err != nil {
		return errors.Annotatef(err, "uploading %s to %s", item.Path, targetURL)
	}
	return nil
}

type progressUpdater struct {
	tomb   tomb.Tomb
	ch     chan progress.Report
	update func(string)
	action string
}

// Sink is part of the progress.Sinker interface.
func (u *progressUpdater) Sink() chan<- progress.Report {
	return u.ch
}

func (u *progressUpdater) loop() {
	defer u.tomb.Done()
	var last float32
	const threshold = 10 // update status every X%
	for {
		select {
		case <-u.tomb.Dying():
			u.tomb.Kill(tomb.ErrDying)
			return
		case report, ok := <-u.ch:
			if !ok {
				return
			}
			var message string
			if err := report.Error(); err != nil {
				message = fmt.Sprintf("%s: %s", u.action, err)
			} else {
				pc := report.Percentage()
				if pc < 100 && (pc-last) < threshold {
					// Don't update yet, to avoid spamming
					// status updates.
					continue
				}
				last = pc
				message = fmt.Sprintf("%s: %.2f%% (%s)", u.action, pc, report.Detail())
			}
			u.update(message)
		}
	}
}

func (u *progressUpdater) done() {
	u.tomb.Kill(nil)
	u.tomb.Wait()
}
