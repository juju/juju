// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"net/url"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/list"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

// Client encapsulates a vSphere client, exposing the subset of
// functionality that we require in the Juju provider.
type Client struct {
	client     *govmomi.Client
	datacenter string
	logger     loggo.Logger
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
	return &Client{client, datacenter, logger}, nil
}

// Close logs out and closes the client connection.
func (c *Client) Close(ctx context.Context) error {
	return c.client.Logout(ctx)
}

func (c *Client) recurser() *list.Recurser {
	return &list.Recurser{
		Collector: property.DefaultCollector(c.client.Client),
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

	// We run all tasks in parallel, and wait for them below.
	var lastError error
	tasks := make([]*object.Task, 0, len(vms)*2)
	for _, vm := range vms {
		c.logger.Debugf("powering off %q", vm.Name())
		task, err := vm.PowerOff(ctx)
		if err != nil {
			lastError = errors.Annotatef(err, "powering off %q", vm.Name())
			c.logger.Errorf(err.Error())
			continue
		}
		tasks = append(tasks, task)

		c.logger.Debugf("destroying %q", vm.Name())
		task, err = vm.Destroy(ctx)
		if err != nil {
			lastError = errors.Annotatef(err, "destroying %q", vm.Name())
			c.logger.Errorf(err.Error())
			continue
		}
		tasks = append(tasks, task)
	}

	for _, task := range tasks {
		_, err := task.WaitForResult(ctx, nil)
		if err != nil {
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

// ComputeResources retuns list of all root compute resources in the system.
func (c *Client) ComputeResources(ctx context.Context) ([]*mo.ComputeResource, error) {
	_, datacenter, err := c.finder(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	root := list.Element{Object: folders.HostFolder}
	es, err := c.recurser().Recurse(ctx, root, []string{"*"})
	if err != nil {
		return nil, err
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

// EnsureVMFolder creates the a VM folder with the given path if it doesn't
// already exist.
func (c *Client) EnsureVMFolder(ctx context.Context, folderPath string) error {
	finder, datacenter, err := c.finder(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	folders, err := datacenter.Folders(ctx)
	if err != nil {
		return errors.Trace(err)
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
			return errors.Annotatef(
				err, "creating folder %q in %q",
				name, parentFolder.InventoryPath,
			)
		}
		parentFolder = folder
	}
	return nil
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
	if _, err := task.WaitForResult(ctx, nil); err != nil {
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
