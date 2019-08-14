// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/list"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"gopkg.in/retry.v1"
)

const (
	// maxExtendWait is the longest we'll wait for a disk to be
	// extended.
	maxExtendWait = 30 * time.Second

	// extendInitialDelay is the shortest time we'll wait for a disk
	// extend to succeed.
	extendInitialDelay = 10 * time.Millisecond

	// extendMaxDelay is the longest we'll wait before checking
	// again whether the disk extend has succeeded.
	extendMaxDelay = 5 * time.Second

	// extendBackoffFactor is the multiple the delay will grow by each
	// attempt.
	extendBackoffFactor = 1.5
)

// ErrExtendDisk is returned if we timed out trying to extend the root
// disk of a VM.
var ErrExtendDisk = errors.Errorf("extending disk failed")

// IsExtendDiskError returns whether the cause of this error was a
// failure to extend a VM disk.
func IsExtendDiskError(err error) bool {
	return errors.Cause(err) == ErrExtendDisk
}

// Client encapsulates a vSphere client, exposing the subset of
// functionality that we require in the Juju provider.
type Client struct {
	client     *govmomi.Client
	datacenter string
	logger     loggo.Logger
	clock      clock.Clock
}

// Dial dials a new vSphere client connection using the given URL,
// scoped to the specified dataceter. The resulting Client's Close
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
		client:     client,
		datacenter: datacenter,
		logger:     logger,
		clock:      clock.WallClock,
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
func (c *Client) Datastores(ctx context.Context) ([]*mo.Datastore, error) {
	_, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	es, err := c.lister(folders.DatastoreFolder.Reference()).List(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var datastores []*mo.Datastore
	for _, e := range es {
		switch o := e.Object.(type) {
		case mo.Datastore:
			datastores = append(datastores, &o)
		}
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

// EnsureVMFolder creates the a VM folder with the given path if it doesn't
// already exist.
func (c *Client) EnsureVMFolder(ctx context.Context, folderPath string) (*object.Folder, error) {
	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	createFolder := func(parent *object.Folder, name string) (*object.Folder, error) {
		folder, err := parent.CreateFolder(ctx, name)
		if err != nil && soap.IsSoapFault(err) {
			switch soap.ToSoapFault(err).VimFault().(type) {
			case types.DuplicateName:
				return finder.Folder(ctx, parent.InventoryPath+"/"+name)
			}
		}
		return folder, err
	}

	parentFolder := folders.VmFolder
	for _, name := range strings.Split(folderPath, "/") {
		folder, err := createFolder(parentFolder, name)
		if err != nil {
			return nil, errors.Annotatef(
				err, "creating folder %q in %q",
				name, parentFolder.InventoryPath,
			)
		}
		parentFolder = folder
	}
	return parentFolder, nil
}

// DestroyVMFolder destroys a folder rooted at the datacenter's base VM folder.
func (c *Client) DestroyVMFolder(ctx context.Context, folderPath string) error {
	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	folderPath = path.Join(folders.VmFolder.InventoryPath, folderPath)
	folder, err := finder.Folder(ctx, folderPath)
	if err != nil {
		if _, ok := err.(*find.NotFoundError); ok {
			return nil
		}
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
	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	parentPath = path.Join(folders.VmFolder.InventoryPath, parentPath)
	childPath = path.Join(folders.VmFolder.InventoryPath, childPath)
	parent, err := finder.Folder(ctx, parentPath)
	if err != nil {
		return errors.Trace(err)
	}
	child, err := finder.Folder(ctx, childPath)
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
	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	folderPath = path.Join(folders.VmFolder.InventoryPath, folderPath)
	folder, err := finder.Folder(ctx, folderPath)
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
) (*object.VirtualMachine, error) {
	_, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, err
	}
	taskWaiter := &taskWaiter{
		args.Clock,
		args.UpdateProgress,
		args.UpdateProgressInterval,
	}

	vmConfigSpec := &types.VirtualMachineConfigSpec{}
	err = c.buildConfigSpec(ctx, args, vmConfigSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newVAppConfig, err := customiseVAppConfig(ctx, srcVM, args)
	if err != nil {
		return nil, errors.Annotate(err, "changing VApp config")
	}
	vmConfigSpec.VAppConfig = newVAppConfig

	task, err := srcVM.Clone(ctx, vmFolder, args.Name, types.VirtualMachineCloneSpec{
		Config: vmConfigSpec,
		Location: types.VirtualMachineRelocateSpec{
			Pool: &args.ResourcePool,
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	info, err := taskWaiter.waitTask(ctx, task, "cloning VM")
	if err != nil {
		return nil, errors.Trace(err)
	}

	vm := object.NewVirtualMachine(c.client.Client, info.Result.(types.ManagedObjectReference))

	if args.Constraints.RootDisk != nil {
		// The user specified a root disk, so extend the VM's
		// disk before powering the VM on.
		args.UpdateProgress(fmt.Sprintf(
			"extending disk to %s",
			humanize.IBytes(megabytesToB(*args.Constraints.RootDisk)),
		))
		if err := c.extendVMRootDisk(
			ctx, vm, datacenter,
			*args.Constraints.RootDisk,
			taskWaiter,
		); err != nil {
			return nil, errors.Trace(err)
		}
	}

	return vm, nil
}

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

func (c *Client) getDiskWithFileBacking(
	ctx context.Context,
	vm *object.VirtualMachine,
) (*types.VirtualDisk, types.BaseVirtualDeviceFileBackingInfo, error) {
	var mo mo.VirtualMachine
	if err := c.client.RetrieveOne(ctx, vm.Reference(), []string{"config.hardware"}, &mo); err != nil {
		return nil, nil, errors.Trace(err)
	}
	for _, dev := range mo.Config.Hardware.Device {
		dev, ok := dev.(*types.VirtualDisk)
		if !ok {
			continue
		}
		backing, ok := dev.Backing.(types.BaseVirtualDeviceFileBackingInfo)
		if !ok {
			continue
		}
		return dev, backing, nil
	}
	return nil, nil, errors.NotFoundf("disk")
}

func (c *Client) extendDisk(
	ctx context.Context,
	vm *object.VirtualMachine,
	datacenter *object.Datacenter,
	datastorePath string,
	capacityKB int64,
	taskWaiter *taskWaiter,
) error {
	// NOTE(axw) there's no ExtendVirtualDisk on the disk manager type,
	// hence why we're dealing with request types directly. Send a patch
	// to govmomi to add this to VirtualDiskManager.

	diskManager := object.NewVirtualDiskManager(c.client.Client)
	dcref := datacenter.Reference()
	req := types.ExtendVirtualDisk_Task{
		This:          diskManager.Reference(),
		Name:          datastorePath,
		Datacenter:    &dcref,
		NewCapacityKb: capacityKB,
	}

	_, err := methods.ExtendVirtualDisk_Task(ctx, c.client.Client, &req)
	if err != nil {
		return errors.Trace(err)
	}
	// NOTE: not waiting for the returned task because it never
	// completes - this is because the API actually returns a
	// permission error when we try to get the status of the task,
	// even though the task is created and runs happily as far as the
	// web client is concerned. (You can see this error by adding
	// logging to govmomi/vim25/methods.WaitForUpdatesEx.) It seems
	// like this is a bug in the API. Instead we wait for the VM's
	// disk's capacity to be what we asked for, or return an error.
	strategy := retry.LimitTime(maxExtendWait, retry.Exponential{
		Initial:  extendInitialDelay,
		Factor:   extendBackoffFactor,
		MaxDelay: extendMaxDelay,
		Jitter:   true,
	})
	a := retry.StartWithCancel(strategy, c.clock, ctx.Done())
	for a.Next() {
		disk, _, err := c.getDiskWithFileBacking(ctx, vm)
		if err != nil {
			return errors.Trace(err)
		}
		if disk.CapacityInKB >= capacityKB {
			return nil
		}
	}
	return errors.Trace(ErrExtendDisk)
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
