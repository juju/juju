// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"context"
	"net/url"
	"sync"

	"github.com/juju/testing"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
)

func newMockDialFunc(dialStub *testing.Stub, client vsphere.Client) vsphere.DialFunc {
	return func(ctx context.Context, u *url.URL, datacenter string) (vsphere.Client, error) {
		dialStub.AddCall("Dial", ctx, u, datacenter)
		if err := dialStub.NextErr(); err != nil {
			return nil, err
		}
		return client, nil
	}
}

type mockClient struct {
	// mu guards testing.Stub access, to ensure that the recorded
	// method calls correspond to the errors returned.
	mu sync.Mutex
	testing.Stub

	computeResources      []*mo.ComputeResource
	resourcePools         map[string][]*object.ResourcePool
	createdVirtualMachine *mo.VirtualMachine
	virtualMachines       []*mo.VirtualMachine
	datastores            []mo.Datastore
	vmFolder              *object.Folder
	hasPrivilege          bool
}

func (c *mockClient) Close(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "Close", ctx)
	return c.NextErr()
}

func (c *mockClient) ComputeResources(ctx context.Context) ([]*mo.ComputeResource, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "ComputeResources", ctx)
	return c.computeResources, c.NextErr()
}

func (c *mockClient) ResourcePools(ctx context.Context, path string) ([]*object.ResourcePool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "ResourcePools", ctx, path)
	return c.resourcePools[path], c.NextErr()
}

func (c *mockClient) CreateVirtualMachine(ctx context.Context, args vsphereclient.CreateVirtualMachineParams) (*mo.VirtualMachine, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "CreateVirtualMachine", ctx, args)
	return c.createdVirtualMachine, c.NextErr()
}

func (c *mockClient) Datastores(ctx context.Context) ([]mo.Datastore, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "Datastores", ctx)
	return c.datastores, c.NextErr()
}

func (c *mockClient) DeleteDatastoreFile(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "DeleteDatastoreFile", ctx, path)
	return c.NextErr()
}

func (c *mockClient) DestroyVMFolder(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "DestroyVMFolder", ctx, path)
	return c.NextErr()
}

func (c *mockClient) FindFolder(ctx context.Context, folderPath string) (vmFolder *object.Folder, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "FindFolder", ctx, folderPath)
	return c.vmFolder, c.NextErr()
}

func (c *mockClient) EnsureVMFolder(ctx context.Context, credAttrFolder string, path string) (*object.Folder, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "EnsureVMFolder", ctx, credAttrFolder, path)
	return c.vmFolder, c.NextErr()
}

func (c *mockClient) MoveVMFolderInto(ctx context.Context, parent string, child string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "MoveVMFolderInto", ctx, parent, child)
	return c.NextErr()
}

func (c *mockClient) MoveVMsInto(ctx context.Context, folder string, vms ...types.ManagedObjectReference) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "MoveVMsInto", ctx, folder, vms)
	return c.NextErr()
}

func (c *mockClient) RemoveVirtualMachines(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "RemoveVirtualMachines", ctx, path)
	return c.NextErr()
}

func (c *mockClient) UpdateVirtualMachineExtraConfig(ctx context.Context, vm *mo.VirtualMachine, attrs map[string]string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "UpdateVirtualMachineExtraConfig", ctx, vm, attrs)
	return c.NextErr()
}

func (c *mockClient) UserHasRootLevelPrivilege(ctx context.Context, privilege string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "UserHasRootLevelPrivilege", ctx, privilege)
	return c.hasPrivilege, c.NextErr()
}

func (c *mockClient) VirtualMachines(ctx context.Context, path string) ([]*mo.VirtualMachine, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MethodCall(c, "VirtualMachines", ctx, path)
	return c.virtualMachines, c.NextErr()
}

func buildVM(name string) *vmBuilder {
	return &vmBuilder{
		name:       name,
		powerState: types.VirtualMachinePowerStatePoweredOn,
		rp:         &types.ManagedObjectReference{},
	}
}

type vmBuilder struct {
	name       string
	powerState types.VirtualMachinePowerState
	nics       []types.GuestNicInfo
	rp         *types.ManagedObjectReference
	metadata   []types.BaseOptionValue
}

func (b *vmBuilder) vm() *mo.VirtualMachine {
	vm := new(mo.VirtualMachine)
	vm.Name = b.name
	vm.Runtime.PowerState = b.powerState
	vm.Guest = &types.GuestInfo{Net: b.nics}
	vm.ResourcePool = b.rp
	vm.Config = &types.VirtualMachineConfigInfo{
		ExtraConfig: b.metadata,
	}
	vm.Self = types.ManagedObjectReference{Value: b.name}
	return vm
}

func (b *vmBuilder) powerOff() *vmBuilder {
	b.powerState = types.VirtualMachinePowerStatePoweredOff
	return b
}

func (b *vmBuilder) nic(nics ...types.GuestNicInfo) *vmBuilder {
	b.nics = append(b.nics, nics...)
	return b
}

func (b *vmBuilder) resourcePool(rp *types.ManagedObjectReference) *vmBuilder {
	b.rp = rp
	return b
}

func (b *vmBuilder) extraConfig(k, v string) *vmBuilder {
	b.metadata = append(b.metadata, &types.OptionValue{Key: k, Value: v})
	return b
}

func newNic(addrs ...string) types.GuestNicInfo {
	return types.GuestNicInfo{IpAddress: addrs}
}

func newComputeResource(name string) *mo.ComputeResource {
	cr := new(mo.ComputeResource)
	cr.Name = name
	cr.ResourcePool = &types.ManagedObjectReference{
		Value: "rp-" + name,
	}
	cr.Summary = &mockSummary{types.ComputeResourceSummary{
		EffectiveCpu: 20,
	}}
	return cr
}

type mockSummary struct {
	types.ComputeResourceSummary
}

func (s *mockSummary) GetComputeResourceSummary() *types.ComputeResourceSummary {
	return &s.ComputeResourceSummary
}

func makeResourcePool(ref, path string) *object.ResourcePool {
	reference := types.ManagedObjectReference{
		Type:  "ResourcePool",
		Value: ref,
	}
	result := object.NewResourcePool(nil, reference)
	result.InventoryPath = path
	return result
}
