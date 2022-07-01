// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/environs"
	"github.com/juju/juju/v3/environs/imagemetadata"
	"github.com/juju/juju/v3/provider/vsphere"
	"github.com/juju/juju/v3/provider/vsphere/internal/ovatest"
	"github.com/juju/juju/v3/provider/vsphere/internal/vsphereclient"
	coretesting "github.com/juju/juju/v3/testing"
)

type vmTemplateSuite struct {
	EnvironFixture
	statusCallbackStub testing.Stub

	statusUpdateParams vsphereclient.StatusUpdateParams
	datastore          *object.Datastore
	mockTemplate       *object.VirtualMachine
}

var _ = gc.Suite(&vmTemplateSuite{})

func (v *vmTemplateSuite) SetUpTest(c *gc.C) {
	v.EnvironFixture.SetUpTest(c)
	v.statusCallbackStub.ResetCalls()
	v.statusUpdateParams = vsphereclient.StatusUpdateParams{
		UpdateProgressInterval: time.Second,
		UpdateProgress:         func(status string) {},
		Clock:                  testclock.NewClock(time.Time{}),
	}
	v.client.folders = makeFolders("/DC/host")
	v.client.computeResources = []vsphereclient.ComputeResource{
		{Resource: newComputeResource("z1"), Path: "/DC/host/z1"},
		{Resource: newComputeResource("z2"), Path: "/DC/host/z2"},
	}
	v.client.resourcePools = map[string][]*object.ResourcePool{
		"/DC/host/z1/...": {makeResourcePool("pool-1", "/DC/host/z1/Resources")},
		"/DC/host/z2/...": {makeResourcePool("pool-2", "/DC/host/z2/Resources")},
	}

	v.client.createdVirtualMachine = buildVM("new-vm").vm()
	v.client.datastores = []mo.Datastore{{
		ManagedEntity: mo.ManagedEntity{Name: "foo"},
	}, {
		ManagedEntity: mo.ManagedEntity{Name: "bar"},
		Summary: types.DatastoreSummary{
			Accessible: true,
		},
	}, {
		ManagedEntity: mo.ManagedEntity{Name: "baz"},
		Summary: types.DatastoreSummary{
			Accessible: true,
		},
	}}
	v.datastore = object.NewDatastore(nil, types.ManagedObjectReference{
		Type:  "Datastore",
		Value: "bar",
	})

	v.mockTemplate = object.NewVirtualMachine(nil, types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: "juju-template-" + ovatest.FakeOVASHA256(),
	})
}

func (v *vmTemplateSuite) addMockLocalTemplateToClient() {
	args := vsphereclient.ImportOVAParameters{
		OVASHA256:    ovatest.FakeOVASHA256(),
		Series:       "trusty",
		Arch:         "amd64",
		TemplateName: "juju-template-" + ovatest.FakeOVASHA256(),
		DestinationFolder: object.NewFolder(nil, types.ManagedObjectReference{
			Type:  "Folder",
			Value: "custom-templates/trusty",
		}),
	}
	v.client.virtualMachineTemplates = []mockTemplateVM{
		{
			vm: object.NewVirtualMachine(nil, types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: args.TemplateName,
			}),
			args: args,
		},
	}
}

func (v *vmTemplateSuite) addMockDownloadedTemplateToClient() {
	v.mockDownloadedTemplateToClient("amd64")
}

func (v *vmTemplateSuite) addMockDownloadedTemplateToClientNoArch() {
	v.mockDownloadedTemplateToClient("")
}

func (v *vmTemplateSuite) mockDownloadedTemplateToClient(arch string) {
	args := vsphereclient.ImportOVAParameters{
		OVASHA256:    ovatest.FakeOVASHA256(),
		Series:       "trusty",
		Arch:         arch,
		TemplateName: "juju-template-" + ovatest.FakeOVASHA256(),
		DestinationFolder: object.NewFolder(nil, types.ManagedObjectReference{
			Type: "Folder",
			// The mocked client does a strings.HasPrefix() on this path when listing templates.
			// We do a greedy search when looking for already imported templates.
			Value: "Juju Controller (deadbeef-1bad-500d-9000-4b1d0d06f00d)/templates/trusty/*",
		}),
	}
	v.client.virtualMachineTemplates = []mockTemplateVM{
		{
			vm: object.NewVirtualMachine(nil, types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: args.TemplateName,
			}),
			args: args,
		},
	}
}

func (v *vmTemplateSuite) TestEnsureTemplateNoImageMetadataSuppliedButImageExistsUpstream(c *gc.C) {
	resPool := v.client.resourcePools["/DC/host/z1/..."][0]
	tplMgr := vsphere.NewVMTemplateManager(
		nil, v.env, v.client, resPool.Reference(),
		v.datastore, v.statusUpdateParams, "",
		coretesting.FakeControllerConfig().ControllerUUID(),
	)

	tpl, arch, err := tplMgr.EnsureTemplate(context.Background(), "trusty", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tpl, gc.NotNil)
	c.Assert(arch, gc.Equals, "amd64")
	v.client.CheckCallNames(c, "ListVMTemplates", "EnsureVMFolder", "CreateTemplateVM")
}

func (v *vmTemplateSuite) TestEnsureTemplateNoImageMetadataSuppliedAndImageDoesNotExistUpstream(c *gc.C) {
	resPool := v.client.resourcePools["/DC/host/z1/..."][0]
	tplMgr := vsphere.NewVMTemplateManager(
		nil, v.env, v.client, resPool.Reference(),
		v.datastore, v.statusUpdateParams, "",
		coretesting.FakeControllerConfig().ControllerUUID(),
	)

	_, _, err := tplMgr.EnsureTemplate(context.Background(), "xenial", "amd64")
	c.Assert(err, jc.Satisfies, environs.IsAvailabilityZoneIndependent)
	c.Assert(err.Error(), gc.Matches, "no matching images found for given constraints.*")
	v.client.CheckCallNames(c, "ListVMTemplates", "EnsureVMFolder")
}

func (v *vmTemplateSuite) TestEnsureTemplateWithImageMetadataSupplied(c *gc.C) {
	imgMeta := []*imagemetadata.ImageMetadata{
		{
			Id:         "custom-templates/trusty",
			RegionName: "/datacenter1",
			Endpoint:   "host1",
			Arch:       "amd64",
		},
	}
	v.addMockLocalTemplateToClient()
	resPool := v.client.resourcePools["/DC/host/z1/..."][0]
	tplMgr := vsphere.NewVMTemplateManager(
		imgMeta, v.env, v.client, resPool.Reference(),
		v.datastore, v.statusUpdateParams, "",
		coretesting.FakeControllerConfig().ControllerUUID(),
	)
	tpl, arch, err := tplMgr.EnsureTemplate(context.Background(), "trusty", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tpl, jc.DeepEquals, v.client.virtualMachineTemplates[0].vm)
	c.Assert(arch, gc.Equals, "amd64")
	v.client.CheckCallNames(c, "ListVMTemplates")
}

func (v *vmTemplateSuite) TestEnsureTemplateImageNotFoundLocally(c *gc.C) {
	imgMeta := []*imagemetadata.ImageMetadata{
		{
			// this image ID does not exist in our mocked templates.
			Id:         "custom-templates/xenial",
			RegionName: "/datacenter1",
			Endpoint:   "host1",
			Arch:       "amd64",
		},
	}

	v.addMockLocalTemplateToClient()
	resPool := v.client.resourcePools["/DC/host/z1/..."][0]
	tplMgr := vsphere.NewVMTemplateManager(
		imgMeta, v.env, v.client, resPool.Reference(),
		v.datastore, v.statusUpdateParams, "",
		coretesting.FakeControllerConfig().ControllerUUID(),
	)
	// trusty exists in the image-download simplestreams
	tpl, arch, err := tplMgr.EnsureTemplate(context.Background(), "trusty", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tpl, gc.NotNil)
	c.Assert(arch, gc.Equals, "amd64")
	// List of calls should be identical to when no custom simplestreams are supplied.
	v.client.CheckCallNames(c, "ListVMTemplates", "ListVMTemplates", "EnsureVMFolder", "CreateTemplateVM")
}

func (v *vmTemplateSuite) TestEnsureTemplateImageCachedImage(c *gc.C) {
	v.addMockDownloadedTemplateToClient()
	resPool := v.client.resourcePools["/DC/host/z1/..."][0]
	tplMgr := vsphere.NewVMTemplateManager(
		nil, v.env, v.client, resPool.Reference(),
		v.datastore, v.statusUpdateParams, "",
		coretesting.FakeControllerConfig().ControllerUUID(),
	)

	tpl, arch, err := tplMgr.EnsureTemplate(context.Background(), "trusty", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tpl, gc.NotNil)
	c.Assert(arch, gc.Equals, "amd64")
	v.client.CheckCallNames(c, "ListVMTemplates", "VirtualMachineObjectToManagedObject")
}

func (v *vmTemplateSuite) TestEnsureTemplateImageCachedImageNoArch(c *gc.C) {
	v.addMockDownloadedTemplateToClientNoArch()
	resPool := v.client.resourcePools["/DC/host/z1/..."][0]
	tplMgr := vsphere.NewVMTemplateManager(
		nil, v.env, v.client, resPool.Reference(),
		v.datastore, v.statusUpdateParams, "",
		coretesting.FakeControllerConfig().ControllerUUID(),
	)

	tpl, arch, err := tplMgr.EnsureTemplate(context.Background(), "trusty", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tpl, gc.NotNil)
	c.Assert(arch, gc.Equals, "")
	v.client.CheckCallNames(c, "ListVMTemplates", "VirtualMachineObjectToManagedObject")
}
