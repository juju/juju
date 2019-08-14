// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"math/big"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mutex"
	"github.com/kr/pretty"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/progress"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/core/constraints"
)

//go:generate go run ../../../../generate/filetoconst/filetoconst.go UbuntuOVF ubuntu.ovf ovf_ubuntu.go 2017 vsphereclient

// NetworkDevice defines a single network device attached to a newly created VM.
type NetworkDevice struct {
	// Network is the name of the network the device should be connected to.
	// If empty it will be connected to the default "VM Network" network.
	Network string
	// MAC is the hardware address of the network device.
	MAC string
}

// That's a default network that's defined in OVF.
const defaultNetwork = "VM Network"

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

	// ResourcePool is a reference to the pool the VM should be
	// created in.
	ResourcePool types.ManagedObjectReference

	// Metadata are metadata key/value pairs to apply to the VM as
	// "extra config".
	Metadata map[string]string

	// Constraints contains the resource constraints for the virtual machine.
	Constraints constraints.Value

	// Networks contain a list of network devices the VM should have.
	NetworkDevices []NetworkDevice

	// UpdateProgress is a function that should be called before/during
	// long-running operations to provide a progress reporting.
	UpdateProgress func(string)

	// UpdateProgressInterval is the amount of time to wait between calls
	// to UpdateProgress. This should be lower when the operation is
	// interactive (bootstrap), and higher when non-interactive.
	UpdateProgressInterval time.Duration

	// Clock is used for controlling the timing of progress updates.
	Clock clock.Clock

	// EnableDiskUUID controls whether the VMware disk should expose a
	// consistent UUID to the guest OS.
	EnableDiskUUID bool
}

// vmTemplateName returns the well-known name to
// the template VM for this controller and its models
func vmTemplateName(args CreateVirtualMachineParams) string {
	return "juju-template-" + args.OVASHA256
}

// vmTemplatePath returns the a path inside the vSphere datastore
// where the template VM is housed.
func vmTemplatePath(args CreateVirtualMachineParams) string {
	return path.Join(args.VMDKDirectory, args.Series)
}

// ensureTemplateVM returns a vSphere template VM
// that instances can be created from.
func (c *Client) ensureTemplateVM(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (vm *object.VirtualMachine, err error) {
	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	templateVM, err := finder.VirtualMachine(ctx, path.Join(vmTemplatePath(args), vmTemplateName(args)))
	if err == nil && templateVM != nil {
		return templateVM, nil
	}
	if _, ok := err.(*find.NotFoundError); !ok {
		return nil, errors.Trace(nil)
	}

	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	datastoreMo, err := c.selectDatastore(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	datastore := object.NewDatastore(c.client.Client, datastoreMo.Reference())
	datastore.DatacenterPath = datacenter.InventoryPath
	datastore.SetInventoryPath(path.Join(folders.DatastoreFolder.InventoryPath, datastoreMo.Name))

	spec, err := c.createImportSpec(ctx, args, datastore)
	if err != nil {
		return nil, errors.Annotate(err, "creating import spec")
	}

	vmFolder, err := c.EnsureVMFolder(ctx, vmTemplatePath(args))
	if err != nil {
		return nil, errors.Trace(err)
	}

	importSpec := spec.ImportSpec
	args.UpdateProgress(fmt.Sprintf("creating template VM %q", vmTemplateName(args)))
	c.logger.Debugf("creating template VM in folder %s", vmFolder)
	c.logger.Tracef("import spec: %s", pretty.Sprint(importSpec))

	// Each controller maintains its own image cache. All compute
	// provisioners (i.e. each model's) run on the same controller
	// machine, so taking a machine lock ensures that only one
	// process is updating VMDKs at the same time. We lock around
	// access to the series directory.
	mutexReleaser, err := mutex.Acquire(mutex.Spec{
		Name:  "juju-vsphere-" + args.Series,
		Clock: args.Clock,
		Delay: time.Second,
	})
	if err != nil {
		return nil, errors.Annotate(err, "acquiring lock")
	}
	defer func() {
		mutexReleaser.Release()
	}()

	resourcePool := object.NewResourcePool(c.client.Client, args.ResourcePool)
	lease, err := resourcePool.ImportVApp(ctx, importSpec, vmFolder, nil)
	if err != nil {
		return nil, errors.Annotate(err, "failed to import vapp")
	}
	info, err := lease.Wait(ctx, spec.FileItem)
	if err != nil {
		return nil, errors.Trace(err)
	}

	updater := lease.StartUpdater(ctx, info)
	defer updater.Done()
	defer func() { // if the connection terminates, propagate the error
		if err != nil {
			abortErr := lease.Abort(ctx, nil)
			if abortErr != nil {
				c.logger.Errorf("error encountered during upload: %s", err)
			}
		}
	}()

	ovaLocation, ovaReadCloser, err := args.ReadOVA()
	if err != nil {
		return nil, errors.Annotate(err, "fetching OVA")
	}
	defer ovaReadCloser.Close()
	sha256sum := sha256.New()
	ovaTarReader := tar.NewReader(io.TeeReader(ovaReadCloser, sha256sum))
	for {
		header, err := ovaTarReader.Next()
		if err != nil {
			return nil, errors.Annotate(err, "reading OVA")
		}
		if strings.HasSuffix(header.Name, ".vmdk") {
			item := info.Items[0]
			c.logger.Infof("Streaming VMDK from %s to %s", ovaLocation, item.URL)
			withStatusUpdater(ctx, "streaming vmdk", args.Clock, args.UpdateProgress, args.UpdateProgressInterval,
				func(ctx context.Context, sink progress.Sinker) {
					opts := soap.Upload{
						ContentLength: header.Size,
						Progress:      sink,
					}

					err = lease.Upload(ctx, item, ovaTarReader, opts)
				},
			)
			if err != nil {
				return nil, errors.Annotatef(
					err, "streaming %s to %s",
					ovaLocation,
					item.URL,
				)
			}

			c.logger.Debugf("VMDK uploaded")
			break
		}
	}
	if _, err := io.Copy(sha256sum, ovaReadCloser); err != nil {
		return nil, errors.Annotate(err, "reading OVA")
	}
	if err := lease.Complete(ctx); err != nil {
		return nil, errors.Trace(err)
	}
	if fmt.Sprintf("%x", sha256sum.Sum(nil)) != args.OVASHA256 {
		return nil, errors.New("SHA-256 hash mismatch for OVA")
	}
	vm = object.NewVirtualMachine(c.client.Client, info.Entity)
	err = vm.MarkAsTemplate(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "marking as template")
	}
	return vm, nil
}

// CreateVirtualMachine creates and powers on a new VM.
//
// The creation process makes use of a vSphere feature called template VMs.
// If it doesn't already exist, a template VM will be created within
// args.VMDKDirectory based off of the OVA data provided by args.ReadOVA.
//
// Once the template VM is available, a new instance will be cloned from
// it. Configuration settings from args.Constraints are then applied through
// a reconfigure step. Once the constraints are applied, the newly cloned VM
// will be powered on.
func (c *Client) CreateVirtualMachine(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (_ *mo.VirtualMachine, resultErr error) {
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
	if err != nil {
		return nil, errors.Trace(err)
	}
	templateVM, err := c.ensureTemplateVM(ctx, args)
	if err != nil {
		return nil, errors.Annotate(err, "creating template VM")
	}

	args.UpdateProgress("cloning template")
	vm, err := c.cloneVM(ctx, args, templateVM, vmFolder)
	if err != nil {
		return nil, errors.Trace(err)
	}

	taskWaiter := &taskWaiter{
		args.Clock,
		args.UpdateProgress,
		args.UpdateProgressInterval,
	}
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
	disk, backing, err := c.getDiskWithFileBacking(ctx, vm)
	if err != nil {
		return errors.Trace(err)
	}
	newCapacityInKB := int64(megabytesToKiB(sizeMB))
	if disk.CapacityInKB >= newCapacityInKB {
		// The root disk is already bigger than the
		// user-specified size, so leave it alone.
		return nil
	}
	datastorePath := backing.GetVirtualDeviceFileBackingInfo().FileName
	return errors.Trace(c.extendDisk(
		ctx, vm, datacenter, datastorePath, newCapacityInKB, taskWaiter,
	))
}

func (c *Client) buildConfigSpec(ctx context.Context,
	args CreateVirtualMachineParams,
	spec *types.VirtualMachineConfigSpec) error {
	if spec == nil {
		spec = &types.VirtualMachineConfigSpec{}
	}
	if args.Constraints.HasCpuCores() {
		spec.NumCPUs = int32(*args.Constraints.CpuCores)
	}
	if args.Constraints.HasMem() {
		spec.MemoryMB = int64(*args.Constraints.Mem)
	}
	if args.Constraints.HasCpuPower() {
		cpuPower := int64(*args.Constraints.CpuPower)
		spec.CpuAllocation = &types.ResourceAllocationInfo{
			Limit:       &cpuPower,
			Reservation: &cpuPower,
		}
	}
	if spec.Flags == nil {
		spec.Flags = &types.VirtualMachineFlagInfo{}
	}
	spec.Flags.DiskUuidEnabled = types.NewBool(args.EnableDiskUUID)

	for k, v := range args.Metadata {
		spec.ExtraConfig = append(spec.ExtraConfig, &types.OptionValue{Key: k, Value: v})
	}

	networks, dvportgroupConfig, err := c.computeResourceNetworks(ctx, args.ComputeResource)
	if err != nil {
		return errors.Trace(err)
	}
	for i, networkDevice := range args.NetworkDevices {
		network := networkDevice.Network
		if network == "" {
			network = defaultNetwork
		}

		networkReference, err := findNetwork(networks, network)
		if err != nil {
			return errors.Trace(err)
		}
		device, err := c.addNetworkDevice(ctx, spec, networkReference, networkDevice.MAC, dvportgroupConfig)
		if err != nil {
			return errors.Annotatef(err, "adding network device %d - network %s", i, network)
		}
		c.logger.Debugf("network device: %+v", device)
	}
	return nil
}

func (c *Client) createImportSpec(
	ctx context.Context,
	args CreateVirtualMachineParams,
	datastore *object.Datastore,
) (*types.OvfCreateImportSpecResult, error) {
	c.logger.Debugf("Creating import spec")
	cisp := types.OvfCreateImportSpecParams{
		EntityName: vmTemplateName(args),
	}

	c.logger.Debugf("Fetching OVF manager")
	ovfManager := ovf.NewManager(c.client.Client)
	spec, err := ovfManager.CreateImportSpec(ctx, UbuntuOVF, args.ResourcePool, datastore, cisp)
	c.logger.Debugf("ImportSpec built")
	if err != nil {
		return nil, errors.Trace(err)
	} else if spec.Error != nil {
		return nil, errors.New(spec.Error[0].LocalizedMessage)
	}
	s := &spec.ImportSpec.(*types.VirtualMachineImportSpec).ConfigSpec
	err = c.buildConfigSpec(ctx, args, s)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return spec, nil
}

func (c *Client) selectDatastore(
	ctx context.Context,
	args CreateVirtualMachineParams,
) (*mo.Datastore, error) {
	c.logger.Debugf("Selecting datastore")
	// Select a datastore. If the user specified one, use that. When no datastore
	// is provided and there is only datastore accessible, use that. Otherwise return
	// an error and ask for guidance.
	refs := make([]types.ManagedObjectReference, len(args.ComputeResource.Datastore))
	for i, ds := range args.ComputeResource.Datastore {
		refs[i] = ds.Reference()
	}
	var datastores []mo.Datastore
	if err := c.client.Retrieve(ctx, refs, nil, &datastores); err != nil {
		return nil, errors.Annotate(err, "retrieving datastore details")
	}

	var accessibleDatastores []mo.Datastore
	var datastoreNames []string
	for _, ds := range datastores {
		if ds.Summary.Accessible {
			accessibleDatastores = append(accessibleDatastores, ds)
			datastoreNames = append(datastoreNames, ds.Name)
		} else {
			c.logger.Debugf("datastore %s is inaccessible", ds.Name)
		}
	}

	if len(accessibleDatastores) == 0 {
		return nil, errors.New("no accessible datastores available")
	}

	if args.Constraints.RootDiskSource != nil {
		dsName := *args.Constraints.RootDiskSource
		c.logger.Debugf("desired datasource %q", dsName)
		c.logger.Debugf("accessible datasources %q", datastoreNames)
		for _, ds := range datastores {
			if ds.Name == dsName {
				c.logger.Infof("selecting datastore %s", ds.Name)
				return &ds, nil
			}
		}
		datastoreNamesMsg := fmt.Sprintf("%q", datastoreNames)
		datastoreNamesMsg = strings.Trim(datastoreNamesMsg, "[]")
		datastoreNames = strings.Split(datastoreNamesMsg, " ")
		datastoreNamesMsg = strings.Join(datastoreNames, ", ")
		return nil, errors.Errorf("could not find datastore %q, datastore(s) accessible: %s", dsName, datastoreNamesMsg)
	}

	if len(accessibleDatastores) == 1 {
		ds := accessibleDatastores[0]
		c.logger.Infof("selecting datastore %s", ds.Name)
		return &ds, nil
	} else if len(accessibleDatastores) > 1 {
		return nil, errors.Errorf("no datastore provided and multiple available: %q", strings.Join(datastoreNames, ", "))
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
	mac string,
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
	if mac != "" {
		if !VerifyMAC(mac) {
			return nil, fmt.Errorf("Invalid MAC address: %q", mac)
		}
		networkDevice.AddressType = "Manual"
		networkDevice.MacAddress = mac
	}
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

// GenerateMAC generates a random hardware address that meets VMWare
// requirements for MAC address: 00:50:56:XX:YY:ZZ where XX is between 00 and 3f.
// https://pubs.vmware.com/vsphere-4-esx-vcenter/index.jsp?topic=/com.vmware.vsphere.server_configclassic.doc_41/esx_server_config/advanced_networking/c_setting_up_mac_addresses.html
func GenerateMAC() (string, error) {
	c, err := rand.Int(rand.Reader, big.NewInt(0x3fffff))
	if err != nil {
		return "", err
	}
	r := c.Uint64()
	return fmt.Sprintf("00:50:56:%.2x:%.2x:%.2x", (r>>16)&0xff, (r>>8)&0xff, r&0xff), nil
}

// VerifyMAC verifies that the MAC is valid for VMWare.
func VerifyMAC(mac string) bool {
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return false
	}
	if parts[0] != "00" || parts[1] != "50" || parts[2] != "56" {
		return false
	}
	for i, part := range parts[3:] {
		v, err := strconv.ParseUint(part, 16, 8)
		if err != nil {
			return false
		}
		if i == 0 && v > 0x3f {
			// 4th byte must be <= 0x3f
			return false
		}
	}
	return true
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
