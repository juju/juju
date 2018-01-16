// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/kr/pretty"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/constraints"
)

//go:generate go run ../../../../generate/filetoconst/filetoconst.go UbuntuOVF ubuntu.ovf ovf_ubuntu.go 2017 vsphereclient

// CreateVirtualMachineParams contains the parameters required for creating
// a new virtual machine.
type CreateVirtualMachineParams struct {
	// Name is the name to give the virtual machine. The VM name is used
	// for its hostname also.
	Name string

	// Folder is the path of the VM folder, relative to the root VM folder,
	// in which to create the VM.
	Folder string

	// VMDKDirectory is the datastore path in which VMDKs are stored for
	// this controller. Within this directory there will be subdirectories
	// for each series, and within those the VMDKs will be stored.
	VMDKDirectory string

	// Series is the name of the OS series that the image will run.
	Series string

	// ReadOVA returns the location of, and an io.ReadCloser for,
	// the OVA from which to extract the VMDK. The location may be
	// used for reporting progress. The ReadCloser must be closed
	// by the caller when it is finished with it.
	ReadOVA func() (location string, _ io.ReadCloser, _ error)

	// OVASHA256 is the expected SHA-256 hash of the OVA.
	OVASHA256 string

	// UserData is the cloud-init user-data.
	UserData string

	// ComputeResource is the compute resource (host or cluster) to be used
	// to create the VM.
	ComputeResource *mo.ComputeResource

	// Datastore is the name of the datastore in which to create the VM.
	// If this is empty, any accessible datastore will be used.
	Datastore string

	// Metadata are metadata key/value pairs to apply to the VM as
	// "extra config".
	Metadata map[string]string

	// Constraints contains the resource constraints for the virtual machine.
	Constraints constraints.Value

	// PrimaryNetwork, if set, is the name of the primary network to which
	// the VM should be connected. If this is empty, the default will be
	// used.
	PrimaryNetwork string

	// ExternalNetwork, if set, is the name of an additional "external"
	// network to which the VM should be connected.
	ExternalNetwork string

	// UpdateProgress is a function that should be called before/during
	// long-running operations to provide a progress reporting.
	UpdateProgress func(string)

	// UpdateProgressInterval is the amount of time to wait between calls
	// to UpdateProgress. This should be lower when the operation is
	// interactive (bootstrap), and higher when non-interactive.
	UpdateProgressInterval time.Duration

	// Clock is used for controlling the timing of progress updates.
	Clock clock.Clock
}

// CreateVirtualMachine creates and powers on a new VM.
//
// This method imports an OVF template using the vSphere API. This process
// comprises the following steps:
//   1. Ensure the VMDK contained within the OVA archive (args.OVA) is
//      stored in the datastore, in this controller's cache. If it is
//      there already, we use it; otherwise we remove any existing VMDK
//      for the same series, and upload the new one.
//   2. Call CreateImportSpec [0] with a pre-canned OVF, which validates
//      the OVF descriptor against the hardware supported by the host system.
//      If the validation succeeds,/the method returns an ImportSpec to use
//      for importing the virtual machine.
//   3. Prepare all necessary parameters (CPU, memory, root disk, etc.), and
//      call the ImportVApp method [0]. This method is responsible for actually
//      creating the VM. This VM is temporary, and used only to convert the
//      VMDK file into a disk type file.
//   4. Clone the temporary VM from step 3, to create the VM we will associate
//      with the Juju machine.
//   5. If the user specified a root-disk constraint, extend the VMDK if its
//      capacity is less than the specified constraint.
//   6. Power on the virtual machine.
//
// [0] https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/
// [1] https://www.vmware.com/support/developer/vc-sdk/visdk41pubs/ApiReference/vim.HttpNfcLease.html
func (c *Client) CreateVirtualMachine(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (_ *mo.VirtualMachine, resultErr error) {

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

	// Select the datastore.
	datastoreMo, err := c.selectDatastore(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	datastore := object.NewDatastore(c.client.Client, datastoreMo.Reference())
	datastore.DatacenterPath = datacenter.InventoryPath
	datastore.SetInventoryPath(path.Join(folders.DatastoreFolder.InventoryPath, datastoreMo.Name))

	// Ensure the VMDK is present in the datastore, uploading it if it
	// doesn't already exist.
	resourcePool := object.NewResourcePool(c.client.Client, *args.ComputeResource.ResourcePool)
	taskWaiter := &taskWaiter{args.Clock, args.UpdateProgress, args.UpdateProgressInterval}
	vmdkDatastorePath, releaseVMDK, err := c.ensureVMDK(ctx, args, datastore, datacenter, taskWaiter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer releaseVMDK()

	// Import the VApp, creating a temporary VM. This is necessary to
	// import the VMDK, which exists in the datastore as a not-a-disk
	// file type.
	args.UpdateProgress("creating import spec")
	importSpec, err := c.createImportSpec(ctx, args, datastore, vmdkDatastorePath)
	if err != nil {
		return nil, errors.Annotate(err, "creating import spec")
	}
	importSpec.ConfigSpec.Name += ".tmp"

	args.UpdateProgress(fmt.Sprintf("creating VM %q", args.Name))
	c.logger.Debugf("creating temporary VM in folder %s", vmFolder)
	c.logger.Tracef("import spec: %s", pretty.Sprint(importSpec))
	lease, err := resourcePool.ImportVApp(ctx, importSpec, vmFolder, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to import vapp")
	}
	info, err := lease.Wait(ctx, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := lease.Complete(ctx); err != nil {
		return nil, errors.Trace(err)
	}
	tempVM := object.NewVirtualMachine(c.client.Client, info.Entity)
	defer func() {
		if err := c.destroyVM(ctx, tempVM, taskWaiter); err != nil {
			c.logger.Warningf("failed to delete temporary VM: %s", err)
		}
	}()

	// Clone the temporary VM to import the VMDK, as mentioned above.
	// After cloning the temporary VM, we must detach the original
	// VMDK from the temporary VM to avoid deleting it when destroying
	// the VM.
	c.logger.Debugf("cloning VM")
	vm, err := c.cloneVM(ctx, tempVM, args.Name, vmFolder, taskWaiter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if resultErr == nil {
			return
		}
		if err := c.destroyVM(ctx, vm, taskWaiter); err != nil {
			c.logger.Warningf("failed to delete VM: %s", err)
		}
	}()
	if _, err := c.detachDisk(ctx, tempVM, taskWaiter); err != nil {
		return nil, errors.Trace(err)
	}

	if args.Constraints.RootDisk != nil {
		// The user specified a root disk, so extend the VM's
		// disk before powering the VM on.
		args.UpdateProgress(fmt.Sprintf(
			"extending disk to %s",
			humanize.IBytes(*args.Constraints.RootDisk*1024*1024),
		))
		if err := c.extendVMRootDisk(
			ctx, vm, datacenter,
			*args.Constraints.RootDisk,
			taskWaiter,
		); err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Finally, power on and return the VM.
	args.UpdateProgress("powering on")
	task, err := vm.PowerOn(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	taskInfo, err := taskWaiter.waitTask(ctx, task, "powering on VM")
	if err != nil {
		return nil, errors.Trace(err)
	}
	var res mo.VirtualMachine
	if err := c.client.RetrieveOne(ctx, *taskInfo.Entity, nil, &res); err != nil {
		return nil, errors.Trace(err)
	}
	return &res, nil
}

func (c *Client) extendVMRootDisk(
	ctx context.Context,
	vm *object.VirtualMachine,
	datacenter *object.Datacenter,
	sizeMB uint64,
	taskWaiter *taskWaiter,
) error {
	var mo mo.VirtualMachine
	if err := c.client.RetrieveOne(ctx, vm.Reference(), []string{"config.hardware"}, &mo); err != nil {
		return errors.Trace(err)
	}
	for _, dev := range mo.Config.Hardware.Device {
		dev, ok := dev.(*types.VirtualDisk)
		if !ok {
			continue
		}
		newCapacityInKB := int64(sizeMB) * 1024
		if dev.CapacityInKB >= newCapacityInKB {
			// The root disk is already bigger than the
			// user-specified size, so leave it alone.
			return nil
		}
		backing, ok := dev.Backing.(types.BaseVirtualDeviceFileBackingInfo)
		if !ok {
			continue
		}
		datastorePath := backing.GetVirtualDeviceFileBackingInfo().FileName
		return errors.Trace(c.extendDisk(
			ctx, datacenter, datastorePath, newCapacityInKB, taskWaiter,
		))
	}
	return errors.New("disk not found")
}

func (c *Client) createImportSpec(
	ctx context.Context,
	args CreateVirtualMachineParams,
	datastore *object.Datastore,
	vmdkDatastorePath string,
) (*types.VirtualMachineImportSpec, error) {
	cisp := types.OvfCreateImportSpecParams{
		EntityName: args.Name,
		PropertyMapping: []types.KeyValue{
			{Key: "user-data", Value: string(args.UserData)},
			{Key: "hostname", Value: string(args.Name)},
		},
	}

	var networks []mo.Network
	var dvportgroupConfig map[types.ManagedObjectReference]types.DVPortgroupConfigInfo
	if args.PrimaryNetwork != "" || args.ExternalNetwork != "" {
		// Fetch the networks available to the compute resource.
		var err error
		networks, dvportgroupConfig, err = c.computeResourceNetworks(ctx, args.ComputeResource)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if args.PrimaryNetwork != "" {
			// The user has specified a network to use. The Ubuntu
			// OVFs define a network called "VM Network"; map that
			// to whatever the user specified.
			network, err := findNetwork(networks, args.PrimaryNetwork)
			if err != nil {
				return nil, errors.Trace(err)
			}
			cisp.NetworkMapping = []types.OvfNetworkMapping{{
				Name:    "VM Network",
				Network: network.Reference(),
			}}
			c.logger.Debugf("VM configured to use network %q: %+v", args.PrimaryNetwork, network)
		}
	}

	ovfManager := ovf.NewManager(c.client.Client)
	resourcePool := object.NewReference(c.client.Client, *args.ComputeResource.ResourcePool)

	spec, err := ovfManager.CreateImportSpec(ctx, UbuntuOVF, resourcePool, datastore, cisp)
	if err != nil {
		return nil, errors.Trace(err)
	} else if spec.Error != nil {
		return nil, errors.New(spec.Error[0].LocalizedMessage)
	}
	importSpec := spec.ImportSpec.(*types.VirtualMachineImportSpec)
	s := &spec.ImportSpec.(*types.VirtualMachineImportSpec).ConfigSpec

	// Apply resource constraints.
	if args.Constraints.HasCpuCores() {
		s.NumCPUs = int32(*args.Constraints.CpuCores)
	}
	if args.Constraints.HasMem() {
		s.MemoryMB = int64(*args.Constraints.Mem)
	}
	if args.Constraints.HasCpuPower() {
		cpuPower := int64(*args.Constraints.CpuPower)
		s.CpuAllocation = &types.ResourceAllocationInfo{
			Limit:       cpuPower,
			Reservation: cpuPower,
		}
	}
	if err := c.addRootDisk(s, args, datastore, vmdkDatastorePath); err != nil {
		return nil, errors.Trace(err)
	}

	// Apply metadata. Note that we do not have the ability set create or
	// apply tags that will show up in vCenter, as that requires a separate
	// vSphere Automation that we do not have an SDK for.
	for k, v := range args.Metadata {
		s.ExtraConfig = append(s.ExtraConfig, &types.OptionValue{Key: k, Value: v})
	}

	if args.ExternalNetwork != "" {
		externalNetwork, err := findNetwork(networks, args.ExternalNetwork)
		if err != nil {
			return nil, errors.Trace(err)
		}
		device, err := c.addNetworkDevice(ctx, s, externalNetwork, dvportgroupConfig)
		if err != nil {
			return nil, errors.Annotate(err, "adding external network device")
		}
		c.logger.Debugf("external network device: %+v", device)
	}
	return importSpec, nil
}

func (c *Client) addRootDisk(
	s *types.VirtualMachineConfigSpec,
	args CreateVirtualMachineParams,
	diskDatastore *object.Datastore,
	vmdkDatastorePath string,
) error {
	for _, d := range s.DeviceChange {
		deviceConfigSpec := d.GetVirtualDeviceConfigSpec()
		existingDisk, ok := deviceConfigSpec.Device.(*types.VirtualDisk)
		if !ok {
			continue
		}
		ds := diskDatastore.Reference()
		disk := &types.VirtualDisk{
			VirtualDevice: types.VirtualDevice{
				Key:           existingDisk.VirtualDevice.Key,
				ControllerKey: existingDisk.VirtualDevice.ControllerKey,
				UnitNumber:    existingDisk.VirtualDevice.UnitNumber,
				Backing: &types.VirtualDiskFlatVer2BackingInfo{
					DiskMode:        string(types.VirtualDiskModePersistent),
					ThinProvisioned: types.NewBool(true),
					VirtualDeviceFileBackingInfo: types.VirtualDeviceFileBackingInfo{
						FileName:  vmdkDatastorePath,
						Datastore: &ds,
					},
				},
			},
		}
		deviceConfigSpec.Device = disk
		deviceConfigSpec.FileOperation = "" // attach existing disk
	}
	return nil
}

func (c *Client) selectDatastore(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (*mo.Datastore, error) {
	// Select a datastore. If the user specified one, use that; otherwise
	// choose the first one in the list that is accessible.
	refs := make([]types.ManagedObjectReference, len(args.ComputeResource.Datastore))
	for i, ds := range args.ComputeResource.Datastore {
		refs[i] = ds.Reference()
	}
	var datastores []mo.Datastore
	if err := c.client.Retrieve(ctx, refs, nil, &datastores); err != nil {
		return nil, errors.Annotate(err, "retrieving datastore details")
	}
	if args.Datastore != "" {
		for _, ds := range datastores {
			if ds.Name == args.Datastore {
				return &ds, nil
			}
		}
		return nil, errors.Errorf("could not find datastore %q", args.Datastore)
	}
	for _, ds := range datastores {
		if ds.Summary.Accessible {
			c.logger.Debugf("using datastore %q", ds.Name)
			return &ds, nil
		}
	}
	return nil, errors.New("could not find an accessible datastore")
}

// addNetworkDevice adds an entry to the VirtualMachineConfigSpec's
// DeviceChange list, to create a NIC device connecting the machine
// to the specified network.
func (c *Client) addNetworkDevice(
	ctx context.Context,
	spec *types.VirtualMachineConfigSpec,
	network *mo.Network,
	dvportgroupConfig map[types.ManagedObjectReference]types.DVPortgroupConfigInfo,
) (*types.VirtualVmxnet3, error) {
	var networkBacking types.BaseVirtualDeviceBackingInfo
	if dvportgroupConfig, ok := dvportgroupConfig[network.Reference()]; !ok {
		// It's not a distributed virtual portgroup, so return
		// a backing info for a plain old network interface.
		networkBacking = &types.VirtualEthernetCardNetworkBackingInfo{
			VirtualDeviceDeviceBackingInfo: types.VirtualDeviceDeviceBackingInfo{
				DeviceName: network.Name,
			},
		}
	} else {
		// It's a distributed virtual portgroup, so retrieve the details of
		// the distributed virtual switch, and return a backing info for
		// connecting the VM to the portgroup.
		var dvs mo.DistributedVirtualSwitch
		if err := c.client.RetrieveOne(
			ctx, *dvportgroupConfig.DistributedVirtualSwitch, nil, &dvs,
		); err != nil {
			return nil, errors.Annotate(err, "retrieving distributed vSwitch details")
		}
		networkBacking = &types.VirtualEthernetCardDistributedVirtualPortBackingInfo{
			Port: types.DistributedVirtualSwitchPortConnection{
				SwitchUuid:   dvs.Uuid,
				PortgroupKey: dvportgroupConfig.Key,
			},
		}
	}

	var networkDevice types.VirtualVmxnet3
	wakeOnLan := true
	networkDevice.WakeOnLanEnabled = &wakeOnLan
	networkDevice.Backing = networkBacking
	networkDevice.Connectable = &types.VirtualDeviceConnectInfo{
		StartConnected:    true,
		AllowGuestControl: true,
	}
	spec.DeviceChange = append(spec.DeviceChange, &types.VirtualDeviceConfigSpec{
		Operation: types.VirtualDeviceConfigSpecOperationAdd,
		Device:    &networkDevice,
	})
	return &networkDevice, nil
}

func findNetwork(networks []mo.Network, name string) (*mo.Network, error) {
	for _, n := range networks {
		if n.Name == name {
			return &n, nil
		}
	}
	return nil, errors.NotFoundf("network %q", name)
}

// computeResourceNetworks returns the networks available to the compute
// resource, and the config info for the distributed virtual portgroup
// networks. Networks are returned with the distributed virtual portgroups
// first, then standard switch networks, and then finally opaque networks.
func (c *Client) computeResourceNetworks(
	ctx context.Context,
	computeResource *mo.ComputeResource,
) ([]mo.Network, map[types.ManagedObjectReference]types.DVPortgroupConfigInfo, error) {
	refsByType := make(map[string][]types.ManagedObjectReference)
	for _, network := range computeResource.Network {
		refsByType[network.Type] = append(refsByType[network.Type], network.Reference())
	}
	var networks []mo.Network
	if refs := refsByType["Network"]; len(refs) > 0 {
		if err := c.client.Retrieve(ctx, refs, nil, &networks); err != nil {
			return nil, nil, errors.Annotate(err, "retrieving network details")
		}
	}
	var opaqueNetworks []mo.OpaqueNetwork
	if refs := refsByType["OpaqueNetwork"]; len(refs) > 0 {
		if err := c.client.Retrieve(ctx, refs, nil, &opaqueNetworks); err != nil {
			return nil, nil, errors.Annotate(err, "retrieving opaque network details")
		}
		for _, on := range opaqueNetworks {
			networks = append(networks, on.Network)
		}
	}
	var dvportgroups []mo.DistributedVirtualPortgroup
	var dvportgroupConfig map[types.ManagedObjectReference]types.DVPortgroupConfigInfo
	if refs := refsByType["DistributedVirtualPortgroup"]; len(refs) > 0 {
		if err := c.client.Retrieve(ctx, refs, nil, &dvportgroups); err != nil {
			return nil, nil, errors.Annotate(err, "retrieving distributed virtual portgroup details")
		}
		dvportgroupConfig = make(map[types.ManagedObjectReference]types.DVPortgroupConfigInfo)
		allnetworks := make([]mo.Network, len(dvportgroups)+len(networks))
		for i, d := range dvportgroups {
			allnetworks[i] = d.Network
			dvportgroupConfig[allnetworks[i].Reference()] = d.Config
		}
		copy(allnetworks[len(dvportgroups):], networks)
		networks = allnetworks
	}
	return networks, dvportgroupConfig, nil
}
