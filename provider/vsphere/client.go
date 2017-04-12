// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"net/url"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
)

// DialFunc is a function type for dialing vSphere client connections.
type DialFunc func(_ context.Context, _ *url.URL, datacenter string) (Client, error)

// Client is an interface for interacting with the vSphere API.
type Client interface {
	Close(context.Context) error
	ComputeResources(context.Context) ([]*mo.ComputeResource, error)
	CreateVirtualMachine(context.Context, vsphereclient.CreateVirtualMachineParams) (*mo.VirtualMachine, error)
	DestroyVMFolder(context.Context, string) error
	EnsureVMFolder(context.Context, string) error
	MoveVMFolderInto(context.Context, string, string) error
	MoveVMsInto(context.Context, string, ...types.ManagedObjectReference) error
	RemoveVirtualMachines(context.Context, string) error
	UpdateVirtualMachineExtraConfig(context.Context, *mo.VirtualMachine, map[string]string) error
	VirtualMachines(context.Context, string) ([]*mo.VirtualMachine, error)
}

func dialClient(
	ctx context.Context,
	cloudSpec environs.CloudSpec,
	dial DialFunc,
) (Client, error) {
	datacenter := cloudSpec.Region
	credAttrs := cloudSpec.Credential.Attributes()
	username := credAttrs[credAttrUser]
	password := credAttrs[credAttrPassword]
	connURL := &url.URL{
		Scheme: "https",
		User:   url.UserPassword(username, password),
		Host:   cloudSpec.Endpoint,
		Path:   "/sdk",
	}
	return dial(ctx, connURL, datacenter)
}
