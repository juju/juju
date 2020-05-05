// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mutex"
	"github.com/kr/pretty"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/list"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// ErrExtendDisk is returned if we timed out trying to extend the root
// disk of a VM.
type extendDiskError struct {
	err error
}

// Error is part of the error interface.
func (e *extendDiskError) Error() string {
	return "extending disk failed: " + e.err.Error()
}

// IsExtendDiskError returns whether the cause of this error was a
// failure to extend a VM disk.
func IsExtendDiskError(err error) bool {
	_, ok := errors.Cause(err).(*extendDiskError)
	return ok
}

// Client encapsulates a vSphere client, exposing the subset of
// functionality that we require in the Juju provider.
type Client struct {
	client       *govmomi.Client
	datacenter   string
	logger       loggo.Logger
	clock        clock.Clock
	acquireMutex func(mutex.Spec) (func(), error)
}

// Dial dials a new vSphere client connection using the given URL,
// scoped to the specified datacenter. The resulting Client's Close
// method must be called in order to release resources allocated by
// Dial.
func Dial(
	ctx context.Context,
	u *url.URL,
	datacenter string,
	logger loggo.Logger,
) (*Client, error) {
	client, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Client{
		client:       client,
		datacenter:   datacenter,
		logger:       logger,
		clock:        clock.WallClock,
		acquireMutex: acquireMutex,
	}, nil
}

// Close logs out and closes the client connection.
func (c *Client) Close(ctx context.Context) error {
	return c.client.Logout(ctx)
}

func (c *Client) lister(ref types.ManagedObjectReference) *list.Lister {
	return &list.Lister{
		Collector: property.DefaultCollector(c.client.Client),
		Reference: ref,
		All:       true,
	}
}

// FindFolder should be able to search for both entire filepaths
// or relative (.) filepaths.
// If the user passes "test" or "/<DC>/vm/test" as folder, it should
// return the pointer for the same folder, and should also deal with
// the case where folderPath is nil or empty.
func (c *Client) FindFolder(ctx context.Context, folderPath string) (vmFolder *object.Folder, err error) {
	if strings.Contains(folderPath, "..") {
		// ".." not supported as per:
		// https://github.com/vmware/govmomi/blob/master/find/finder.go#L114
		c.logger.Errorf("vm folder path %q contains %q which is not supported", folderPath, "..")
		return nil, errors.NotSupportedf("vm folder path contains ..")
	}

	fi, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	dcfolders, err := datacenter.Folders(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if folderPath == "" {
		c.logger.Warningf("empty string passed as vm-folder, using Datacenter root folder instead")
		return dcfolders.VmFolder, nil
	}

	folderPath = strings.TrimPrefix(folderPath, "./")
	if !strings.HasPrefix(folderPath, dcfolders.VmFolder.InventoryPath) {
		c.logger.Debugf("relative folderPath %q found, join with DC vm folder %q now", folderPath, dcfolders.VmFolder.InventoryPath)
		folderPath = path.Join(dcfolders.VmFolder.InventoryPath, folderPath)
	}

	vmFolder, err = fi.Folder(ctx, folderPath)
	if err == nil {
		return vmFolder, nil
	}
	if _, ok := err.(*find.NotFoundError); ok {
		return nil, errors.NotFoundf("folder path %q", folderPath)
	}
	return nil, errors.Trace(err)
}

func (c *Client) finder(ctx context.Context) (*find.Finder, *object.Datacenter, error) {
	finder := find.NewFinder(c.client.Client, true)
	datacenter, err := finder.Datacenter(ctx, c.datacenter)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	finder.SetDatacenter(datacenter)
	return finder, datacenter, nil
}

// RemoveVirtualMachines removes VMs matching the given path from the
// system. The path may include wildcards, to match multiple VMs.
func (c *Client) RemoveVirtualMachines(ctx context.Context, path string) error {
	finder, _, err := c.finder(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	vms, err := finder.VirtualMachineList(ctx, path)
	if err != nil {
		if _, ok := err.(*find.NotFoundError); ok {
			c.logger.Debugf("no VMs matching path %q", path)
			return nil
		}
		return errors.Annotatef(err, "listing VMs at %q", path)
	}

	// Retrieve VM details so we know which ones to power off.
	refs := make([]types.ManagedObjectReference, len(vms))
	for i, vm := range vms {
		refs[i] = vm.Reference()
	}
	var mos []mo.VirtualMachine
	if err := c.client.Retrieve(ctx, refs, nil, &mos); err != nil {
		return errors.Annotate(err, "retrieving VM details")
	}

	// We run all tasks in parallel, and wait for them below.
	var lastError error
	tasks := make([]*object.Task, 0, len(vms)*2)
	for i, vm := range vms {
		if mos[i].Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn {
			c.logger.Debugf("powering off %q", vm.Name())
			task, err := vm.PowerOff(ctx)
			if err != nil {
				lastError = errors.Annotatef(err, "powering off %q", vm.Name())
				c.logger.Errorf(err.Error())
				continue
			}
			tasks = append(tasks, task)
		}
		c.logger.Debugf("destroying %q", vm.Name())
		task, err := vm.Destroy(ctx)
		if err != nil {
			lastError = errors.Annotatef(err, "destroying %q", vm.Name())
			c.logger.Errorf(err.Error())
			continue
		}
		tasks = append(tasks, task)
	}

	for _, task := range tasks {
		_, err := task.WaitForResult(ctx, nil)
		if err != nil && !isManagedObjectNotFound(err) {
			lastError = err
			c.logger.Errorf(err.Error())
		}
	}
	return errors.Annotate(lastError, "failed to remove instances")
}

// VirtualMachines return list of all VMs in the system matching the given path.
func (c *Client) VirtualMachines(ctx context.Context, path string) ([]*mo.VirtualMachine, error) {
	finder, _, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	items, err := finder.VirtualMachineList(ctx, path)
	if err != nil {
		if _, ok := err.(*find.NotFoundError); ok {
			return nil, nil
		}
		return nil, errors.Annotate(err, "listing VMs")
	}

	vms := make([]*mo.VirtualMachine, len(items))
	for i, item := range items {
		var vm mo.VirtualMachine
		err := c.client.RetrieveOne(ctx, item.Reference(), nil, &vm)
		if err != nil {
			return nil, errors.Trace(err)
		}
		vms[i] = &vm
	}
	return vms, nil
}

// ComputeResources returns list of all root compute resources in the system.
func (c *Client) ComputeResources(ctx context.Context) ([]*mo.ComputeResource, error) {
	_, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	es, err := c.lister(folders.HostFolder.Reference()).List(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cprs []*mo.ComputeResource
	for _, e := range es {
		switch o := e.Object.(type) {
		case mo.ClusterComputeResource:
			cprs = append(cprs, &o.ComputeResource)
		case mo.ComputeResource:
			cprs = append(cprs, &o)
		}
	}
	return cprs, nil
}

// Datastores returns list of all datastores in the system.
func (c *Client) Datastores(ctx context.Context) ([]mo.Datastore, error) {
	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	dsPath := path.Join(folders.DatastoreFolder.InventoryPath, "...")
	c.logger.Tracef("listing datastores under %q", dsPath)
	items, err := finder.DatastoreList(ctx, dsPath)
	if err != nil {
		if _, ok := err.(*find.NotFoundError); ok {
			c.logger.Debugf("no datastores for path %q", dsPath)
			return nil, nil
		}
		return nil, errors.Trace(err)
	}

	refs := make([]types.ManagedObjectReference, len(items))
	for i, item := range items {
		c.logger.Tracef("%s", item.InventoryPath)
		refs[i] = item.Reference()
	}

	var datastores []mo.Datastore
	err = c.client.Retrieve(ctx, refs, nil, &datastores)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving datastore details")
	}
	return datastores, nil
}

// ResourcePools returns a list of all of the resource pools (possibly
// nested) under the given path.
func (c *Client) ResourcePools(ctx context.Context, path string) ([]*object.ResourcePool, error) {
	finder, _, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.logger.Tracef("listing resource pools under %q", path)
	items, err := finder.ResourcePoolList(ctx, path)
	if err != nil {
		if _, ok := err.(*find.NotFoundError); ok {
			c.logger.Debugf("no resource pools for path %q", path)
			return nil, nil
		}
		return nil, errors.Annotate(err, "listing resource pools")
	}
	return items, nil
}

// EnsureVMFolder creates the a VM folder with the given path if it doesn't already exist.
// Two string arguments needed: relativeFolderPath will be split on "/"
// whereas parentFolderName is the subfolder in DC's root-folder.
// The parentFolderName will fallback to DC's root-folder if it's an empty string.
func (c *Client) EnsureVMFolder(ctx context.Context, parentFolderName string, relativeFolderPath string) (*object.Folder, error) {

	finder, _, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	createFolder := func(parent *object.Folder, name string) (*object.Folder, error) {
		getFolder := func() (*object.Folder, error) {
			fd, err := finder.Folder(ctx, path.Join(parent.InventoryPath, name))
			if err != nil {
				return nil, errors.Trace(err)
			}
			return fd, nil
		}
		fd, err := parent.CreateFolder(ctx, name)
		if err == nil {
			return fd, nil
		}
		if soap.IsSoapFault(err) {
			switch soap.ToSoapFault(err).VimFault().(type) {
			case types.DuplicateName:
				return getFolder()
			}
		}
		return nil, errors.Trace(err)
	}

	parentFolder, err := c.FindFolder(ctx, parentFolderName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Creating "Juju Controller (...)" folder and then model folder, for example.
	for _, name := range strings.Split(relativeFolderPath, "/") {
		folder, err := createFolder(parentFolder, name)
		if err != nil {
			return nil, errors.Annotatef(err, "creating folder %q in %q", name, parentFolder.InventoryPath)
		}
		parentFolder = folder
	}
	return parentFolder, nil
}

// DestroyVMFolder destroys a folder(folderPath could be either relative path of vmfolder of datacenter or full path).
func (c *Client) DestroyVMFolder(ctx context.Context, folderPath string) error {
	folder, err := c.FindFolder(ctx, folderPath)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}

	task, err := folder.Destroy(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = task.WaitForResult(ctx, nil)
	if err != nil && !isManagedObjectNotFound(err) {
		return errors.Trace(err)
	}
	return nil
}

// MoveVMFolderInto moves one VM folder into another.
func (c *Client) MoveVMFolderInto(ctx context.Context, parentPath, childPath string) error {
	parent, err := c.FindFolder(ctx, parentPath)
	if err != nil {
		return errors.Trace(err)
	}
	child, err := c.FindFolder(ctx, childPath)
	if err != nil {
		return errors.Trace(err)
	}

	task, err := parent.MoveInto(ctx, []types.ManagedObjectReference{child.Reference()})
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := task.WaitForResult(ctx, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// MoveVMsInto moves a set of VMs into a folder.
func (c *Client) MoveVMsInto(
	ctx context.Context,
	folderPath string,
	vms ...types.ManagedObjectReference,
) error {
	folder, err := c.FindFolder(ctx, folderPath)
	if err != nil {
		return errors.Trace(err)
	}

	task, err := folder.MoveInto(ctx, vms)
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := task.WaitForResult(ctx, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// UpdateVirtualMachineExtraConfig updates the "ExtraConfig" attributes
// of the specified virtual machine. Keys with empty values will be
// removed from the config; existing keys that are unspecified in the
// map will be untouched.
func (c *Client) UpdateVirtualMachineExtraConfig(
	ctx context.Context,
	vmInfo *mo.VirtualMachine,
	metadata map[string]string,
) error {
	var spec types.VirtualMachineConfigSpec
	for k, v := range metadata {
		opt := &types.OptionValue{Key: k, Value: v}
		spec.ExtraConfig = append(spec.ExtraConfig, opt)
	}
	vm := object.NewVirtualMachine(c.client.Client, vmInfo.Reference())
	task, err := vm.Reconfigure(ctx, spec)
	if err != nil {
		return errors.Annotate(err, "reconfiguring VM")
	}
	if _, err := task.WaitForResult(ctx, nil); err != nil {
		return errors.Annotate(err, "reconfiguring VM")
	}
	return nil
}

// DeleteDatastoreFile deletes a file or directory in the datastore.
func (c *Client) DeleteDatastoreFile(ctx context.Context, datastorePath string) error {
	_, datacenter, err := c.finder(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	fileManager := object.NewFileManager(c.client.Client)
	deleteTask, err := fileManager.DeleteDatastoreFile(ctx, datastorePath, datacenter)
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := deleteTask.WaitForResult(ctx, nil); err != nil {
		if types.IsFileNotFound(err) {
			return nil
		}
		return errors.Trace(err)
	}
	return nil
}

// megabytesToB converts the root disk constraint (which uses megabytes for its unit) to bytes
func megabytesToB(diskMiB uint64) uint64 {
	return diskMiB * 1024 * 1024
}

// megabytesToKiB converts the root disk constraint (which uses megabytes for its unit) to kibibytes
func megabytesToKiB(diskMiB uint64) uint64 {
	return diskMiB * 1024
}

func (c *Client) cloneVM(
	ctx context.Context,
	args CreateVirtualMachineParams,
	srcVM *object.VirtualMachine,
	vmFolder *object.Folder,
	datastore *object.Datastore,
) (*object.VirtualMachine, error) {
	taskWaiter := &taskWaiter{
		args.Clock,
		args.UpdateProgress,
		args.UpdateProgressInterval,
	}

	vmConfigSpec, err := c.buildConfigSpec(ctx, args, srcVM)
	if err != nil {
		return nil, errors.Annotate(err, "building clone VM config")
	}
	datastoreRef := datastore.Reference()
	task, err := srcVM.Clone(ctx, vmFolder, args.Name, types.VirtualMachineCloneSpec{
		Config: vmConfigSpec,
		Location: types.VirtualMachineRelocateSpec{
			Pool:      &args.ResourcePool,
			Datastore: &datastoreRef,
		},
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cloning VM %q from %q", args.Name, srcVM.Name())
	}
	info, err := taskWaiter.waitTask(ctx, task, "cloning VM")
	if err != nil {
		return nil, errors.Annotatef(err, "waiting for task %q", info.Name)
	}

	vm := object.NewVirtualMachine(c.client.Client, info.Result.(types.ManagedObjectReference))
	return vm, nil
}

func (c *Client) buildConfigSpec(
	ctx context.Context,
	args CreateVirtualMachineParams,
	srcVM *object.VirtualMachine,
) (*types.VirtualMachineConfigSpec, error) {
	var spec types.VirtualMachineConfigSpec
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

	spec.Flags = &types.VirtualMachineFlagInfo{
		DiskUuidEnabled: types.NewBool(args.EnableDiskUUID),
	}

	for k, v := range args.Metadata {
		spec.ExtraConfig = append(spec.ExtraConfig, &types.OptionValue{Key: k, Value: v})
	}

	networks, dvportgroupConfig, err := c.computeResourceNetworks(ctx, args.ComputeResource)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for i, networkDevice := range args.NetworkDevices {
		network := networkDevice.Network
		if network == "" {
			network = defaultNetwork
		}

		networkReference, err := findNetwork(networks, network)
		if err != nil {
			return nil, errors.Trace(err)
		}
		device, err := c.addNetworkDevice(ctx, &spec, networkReference, networkDevice.MAC, dvportgroupConfig)
		if err != nil {
			return nil, errors.Annotatef(err, "adding network device %d - network %s", i, network)
		}
		c.logger.Debugf("network device: %+v", device)
	}
	newVAppConfig, err := customiseVAppConfig(ctx, srcVM, args)
	if err != nil {
		return nil, errors.Annotate(err, "changing VApp config")
	}
	spec.VAppConfig = newVAppConfig

	return &spec, nil
}

// customiseVAppConfig gets the existing VApp config properties from
// the template VM passed in and uses them to construct edits for the
// new cloned VM.
func customiseVAppConfig(
	ctx context.Context,
	srcVM *object.VirtualMachine,
	args CreateVirtualMachineParams,
) (*types.VmConfigSpec, error) {
	var vmProps mo.VirtualMachine
	if err := srcVM.Properties(ctx, srcVM.Reference(), []string{"config.vAppConfig"}, &vmProps); err != nil {
		return nil, errors.Annotate(err, "getting vAppConfig from template")
	}

	hostnameKey := int32(-1)
	userDataKey := int32(-1)

	vmConfigInfo := vmProps.Config.VAppConfig.GetVmConfigInfo()
	for _, property := range vmConfigInfo.Property {
		switch property.Id {
		case "hostname":
			hostnameKey = property.Key
		case "user-data":
			userDataKey = property.Key
		}
	}

	if hostnameKey == -1 {
		return nil, errors.Errorf("couldn't find hostname property on template %q", srcVM.InventoryPath)
	}
	if userDataKey == -1 {
		return nil, errors.Errorf("couldn't find user-data property on template %q", srcVM.InventoryPath)
	}

	return &types.VmConfigSpec{
		Property: []types.VAppPropertySpec{{
			ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
			Info:            &types.VAppPropertyInfo{Key: hostnameKey, Value: args.Name},
		}, {
			ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
			Info:            &types.VAppPropertyInfo{Key: userDataKey, Value: args.UserData},
		}},
	}, nil
}

func (c *Client) getDisk(
	ctx context.Context,
	vm *object.VirtualMachine,
) (*types.VirtualDisk, error) {
	var mo mo.VirtualMachine
	if err := c.client.RetrieveOne(ctx, vm.Reference(), []string{"config.hardware"}, &mo); err != nil {
		return nil, errors.Trace(err)
	}
	for _, dev := range mo.Config.Hardware.Device {
		if dev, ok := dev.(*types.VirtualDisk); ok {
			return dev, nil
		}
	}
	return nil, errors.NotFoundf("disk")
}

func (c *Client) extendDisk(
	ctx context.Context,
	vm *object.VirtualMachine,
	disk *types.VirtualDisk,
	desiredCapacityKB int64,
) error {
	prettySize := func(kb int64) string { return humanize.IBytes(uint64(kb) * 1024) }
	c.logger.Debugf("extending disk from %q to %q", prettySize(disk.CapacityInKB), prettySize(desiredCapacityKB))

	// Resize the disk to desired size.
	disk.CapacityInKB = desiredCapacityKB

	spec := types.VirtualMachineConfigSpec{}
	spec.DeviceChange = append(spec.DeviceChange, &types.VirtualDeviceConfigSpec{
		Device:        disk,
		Operation:     types.VirtualDeviceConfigSpecOperationEdit,
		FileOperation: "",
	})
	c.logger.Tracef("extending disk, config change -> %s", pretty.Sprint(spec))
	task, err := vm.Reconfigure(ctx, spec)
	if err != nil {
		return errors.Trace(&extendDiskError{err})
	}
	if err := task.Wait(ctx); err != nil {
		return errors.Trace(&extendDiskError{err})
	}
	return nil
}

func isManagedObjectNotFound(err error) bool {
	if err == nil {
		return false
	}
	if f, ok := err.(types.HasFault); ok {
		switch f.Fault().(type) {
		case *types.ManagedObjectNotFound:
			return true
		}
	}
	return false
}

// UserHasRootLevelPrivilege returns whether the connected user has the
// specified privilege on the root-level object.
func (c *Client) UserHasRootLevelPrivilege(ctx context.Context, privilege string) (bool, error) {
	session, err := c.client.SessionManager.UserSession(ctx)
	if err != nil {
		return false, errors.Annotate(err, "getting user session")
	}
	vimClient := c.client.Client
	req := types.HasPrivilegeOnEntities{
		This:      *vimClient.ServiceContent.AuthorizationManager,
		Entity:    []types.ManagedObjectReference{vimClient.ServiceContent.RootFolder},
		SessionId: session.Key,
		PrivId:    []string{privilege},
	}

	resp, err := methods.HasPrivilegeOnEntities(ctx, vimClient, &req)
	if privilege == "System.Read" && isPermissionError(err) {
		// This is a special case - for System.Read you need the
		// privilege to check whether you have the privilege.
		return false, nil
	} else if err != nil {
		return false, errors.Annotatef(err, "checking for %q privilege", privilege)
	}

	if count := len(resp.Returnval); count != 1 {
		return false, errors.Errorf("expected 1 privilege response, got %d", count)
	}
	entityPriv := resp.Returnval[0]
	if count := len(entityPriv.PrivAvailability); count != 1 {
		return false, errors.Errorf("expected 1 privilege availability, got %d", count)
	}

	return entityPriv.PrivAvailability[0].IsGranted, nil
}

func isPermissionError(err error) bool {
	if err == nil || !soap.IsSoapFault(err) {
		return false
	}
	switch soap.ToSoapFault(err).VimFault().(type) {
	case types.NoPermission:
		return true
	}
	return false
}
