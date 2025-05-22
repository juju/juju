// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/vsphere"
	"github.com/juju/juju/internal/provider/vsphere/internal/ovatest"
	"github.com/juju/juju/internal/provider/vsphere/internal/vsphereclient"
	"github.com/juju/juju/internal/provider/vsphere/mocks"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

type legacyEnvironBrokerSuite struct {
	EnvironFixture
	statusCallbackStub testhelpers.Stub
}

func TestLegacyEnvironBrokerSuite(t *testing.T) {
	tc.Run(t, &legacyEnvironBrokerSuite{})
}
func (s *legacyEnvironBrokerSuite) SetUpTest(c *tc.C) {
	s.EnvironFixture.SetUpTest(c)
	s.statusCallbackStub.ResetCalls()

	s.client.folders = makeFolders("/DC/host")
	s.client.computeResources = []vsphereclient.ComputeResource{
		{Resource: newComputeResource("z1"), Path: "/DC/host/z1"},
		{Resource: newComputeResource("z2"), Path: "/DC/host/z2"},
	}
	s.client.resourcePools = map[string][]*object.ResourcePool{
		"/DC/host/z1/...": {makeResourcePool("pool-1", "/DC/host/z1/Resources")},
		"/DC/host/z2/...": {makeResourcePool("pool-2", "/DC/host/z2/Resources")},
	}

	s.client.createdVirtualMachine = buildVM("new-vm").vm()
}

func (s *legacyEnvironBrokerSuite) createStartInstanceArgs(c *tc.C) environs.StartInstanceParams {
	var cons constraints.Value
	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(
		coretesting.FakeControllerConfig(), cons, cons, corebase.MakeDefaultBase("ubuntu", "22.04"), "", nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	setInstanceConfigAuthorizedKeys(c, instanceConfig)
	tools := setInstanceConfigTools(c, instanceConfig)

	return environs.StartInstanceParams{
		AvailabilityZone: "z1",
		ControllerUUID:   instanceConfig.ControllerConfig.ControllerUUID(),
		InstanceConfig:   instanceConfig,
		Tools:            tools,
		Constraints:      cons,
		StatusCallback: func(ctx context.Context, status status.Status, info string, data map[string]interface{}) error {
			s.statusCallbackStub.AddCall("StatusCallback", status, info, data)
			return s.statusCallbackStub.NextErr()
		},
	}
}

func setInstanceConfigTools(c *tc.C, instanceConfig *instancecfg.InstanceConfig) coretools.List {
	tools := []*coretools.Tools{{
		Version: semversion.Binary{
			Number:  semversion.MustParse("1.2.3"),
			Arch:    arch.AMD64,
			Release: "ubuntu",
		},
		URL: "https://example.org",
	}}
	err := instanceConfig.SetTools(tools[:1])
	c.Assert(err, tc.ErrorIsNil)
	return tools
}

func setInstanceConfigAuthorizedKeys(c *tc.C, instanceConfig *instancecfg.InstanceConfig) {
	config := fakeConfig(c)
	instanceConfig.AuthorizedKeys = config.AuthorizedKeys()
}

func (s *legacyEnvironBrokerSuite) TestStartInstance(c *tc.C) {
	s.client.datastores = []mo.Datastore{{
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

	startInstArgs := s.createStartInstanceArgs(c)
	startInstArgs.InstanceConfig.Tags = map[string]string{
		"k0": "v0",
		"k1": "v1",
	}

	result, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)
	c.Assert(result.Instance, tc.NotNil)
	c.Assert(result.Instance.Id(), tc.Equals, instance.Id("new-vm"))

	s.client.CheckCallNames(c, "Folders", "ComputeResources", "ResourcePools", "ResourcePools", "GetTargetDatastore", "ListVMTemplates", "EnsureVMFolder", "CreateTemplateVM", "CreateVirtualMachine", "Close")
	call := s.client.Calls()[8]
	c.Assert(call.Args, tc.HasLen, 2)
	c.Assert(call.Args[0], tc.Implements, new(context.Context))
	c.Assert(call.Args[1], tc.FitsTypeOf, vsphereclient.CreateVirtualMachineParams{})

	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.UserData, tc.Not(tc.Equals), "")

	createVMArgs.UserData = ""
	createVMArgs.Constraints = constraints.Value{}
	createVMArgs.StatusUpdateParams.UpdateProgress = nil
	createVMArgs.StatusUpdateParams.Clock = nil

	createVMArgs.NetworkDevices = []vsphereclient.NetworkDevice{}
	c.Assert(createVMArgs, tc.DeepEquals, vsphereclient.CreateVirtualMachineParams{
		Name:            "juju-f75cba-0",
		Folder:          `Juju Controller (deadbeef-1bad-500d-9000-4b1d0d06f00d)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
		Metadata:        startInstArgs.InstanceConfig.Tags,
		ComputeResource: s.client.computeResources[0].Resource,
		ResourcePool: types.ManagedObjectReference{
			Type:  "ResourcePool",
			Value: "pool-1",
		},
		StatusUpdateParams: vsphereclient.StatusUpdateParams{
			UpdateProgressInterval: 5 * time.Second,
		},
		EnableDiskUUID: true,
		Datastore: object.NewDatastore(nil, types.ManagedObjectReference{
			Type:  "Datastore",
			Value: "bar",
		}),
		VMTemplate: object.NewVirtualMachine(nil, types.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: "juju-template-" + ovatest.FakeOVASHA256(),
		}),
		DiskProvisioningType: vsphereclient.DefaultDiskProvisioningType,
	})
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceNetwork(c *tc.C) {
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"primary-network":    "foo",
			"external-network":   "bar",
			"image-metadata-url": s.imageServer.URL,
		}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	result, err := env.StartInstance(c.Context(), s.createStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)

	call := s.client.Calls()[8]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.NetworkDevices, tc.HasLen, 2)
	c.Assert(createVMArgs.NetworkDevices[0].Network, tc.Equals, "foo")
	c.Assert(createVMArgs.NetworkDevices[1].Network, tc.Equals, "bar")
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceDiskProvisioningMissingModelConfigOption(c *tc.C) {
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"image-metadata-url": s.imageServer.URL,
		}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	result, err := env.StartInstance(c.Context(), s.createStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)

	call := s.client.Calls()[8]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.DiskProvisioningType, tc.Equals, vsphereclient.DefaultDiskProvisioningType)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceDiskProvisioningDefaultOption(c *tc.C) {
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"image-metadata-url":     s.imageServer.URL,
			"disk-provisioning-type": "",
		}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	result, err := env.StartInstance(c.Context(), s.createStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)

	call := s.client.Calls()[8]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.DiskProvisioningType, tc.Equals, vsphereclient.DefaultDiskProvisioningType)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceDiskProvisioningThinDisk(c *tc.C) {
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"image-metadata-url":     s.imageServer.URL,
			"disk-provisioning-type": "thin",
		}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	result, err := env.StartInstance(c.Context(), s.createStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)

	call := s.client.Calls()[8]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.DiskProvisioningType, tc.Equals, vsphereclient.DiskTypeThin)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceDiskProvisioningThickDisk(c *tc.C) {
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"image-metadata-url":     s.imageServer.URL,
			"disk-provisioning-type": "thick",
		}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	result, err := env.StartInstance(c.Context(), s.createStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)

	call := s.client.Calls()[8]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.DiskProvisioningType, tc.Equals, vsphereclient.DiskTypeThick)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceDiskProvisioningThickEagerZeroDisk(c *tc.C) {
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"image-metadata-url":     s.imageServer.URL,
			"disk-provisioning-type": "thick",
		}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	result, err := env.StartInstance(c.Context(), s.createStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)

	call := s.client.Calls()[8]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.DiskProvisioningType, tc.Equals, vsphereclient.DiskTypeThick)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceLongModelName(c *tc.C) {
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"name":               "supercalifragilisticexpialidocious",
			"image-metadata-url": s.imageServer.URL,
		}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	startInstArgs := s.createStartInstanceArgs(c)
	_, err = env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	call := s.client.Calls()[8]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	// The model name in the folder name should be truncated
	// so that the final part of the model name is 80 characters.
	c.Assert(path.Base(createVMArgs.Folder), tc.HasLen, 80)
	c.Assert(createVMArgs.Folder, tc.Equals,
		`Juju Controller (deadbeef-1bad-500d-9000-4b1d0d06f00d)/Model "supercalifragilisticexpialidociou" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceDiskUUIDDisabled(c *tc.C) {
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"enable-disk-uuid":   false,
			"image-metadata-url": s.imageServer.URL,
		}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	result, err := env.StartInstance(c.Context(), s.createStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)

	call := s.client.Calls()[8]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.EnableDiskUUID, tc.Equals, false)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceWithUnsupportedConstraints(c *tc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	startInstArgs.Tools[0].Version.Arch = "someArch"
	_, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorMatches, "no matching images found for given constraints: .*")
	c.Assert(err, tc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceDefaultConstraintsApplied(c *tc.C) {
	cfg := s.env.Config()
	cfg, err := cfg.Apply(map[string]interface{}{
		"datastore": "datastore0",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	startInstArgs := s.createStartInstanceArgs(c)
	res, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorIsNil)

	var (
		arch      = "amd64"
		rootDisk  = common.MinRootDiskSizeGiB(ostype.Ubuntu) * 1024
		datastore = "datastore0"
	)
	c.Assert(res.Hardware, tc.DeepEquals, &instance.HardwareCharacteristics{
		Arch:           &arch,
		RootDisk:       &rootDisk,
		RootDiskSource: &datastore,
	})
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceCustomConstraintsApplied(c *tc.C) {
	var (
		cpuCores uint64 = 4
		cpuPower uint64 = 2001
		mem      uint64 = 2002
		rootDisk uint64 = 10003
		source          = "datastore1"
	)
	startInstArgs := s.createStartInstanceArgs(c)
	startInstArgs.Constraints.CpuCores = &cpuCores
	startInstArgs.Constraints.CpuPower = &cpuPower
	startInstArgs.Constraints.Mem = &mem
	startInstArgs.Constraints.RootDisk = &rootDisk
	startInstArgs.Constraints.RootDiskSource = &source

	res, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorIsNil)

	defaultArch := arch.DefaultArchitecture
	c.Assert(res.Hardware, tc.DeepEquals, &instance.HardwareCharacteristics{
		Arch:           &defaultArch,
		CpuCores:       &cpuCores,
		CpuPower:       &cpuPower,
		Mem:            &mem,
		RootDisk:       &rootDisk,
		RootDiskSource: &source,
	})
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceCallsFinishMachineConfig(c *tc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	s.PatchValue(&vsphere.FinishInstanceConfig, func(mcfg *instancecfg.InstanceConfig, cfg *config.Config) (err error) {
		return errors.New("FinishMachineConfig called")
	})
	_, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorMatches, "FinishMachineConfig called")
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceDefaultDiskSizeIsUsedForSmallConstraintValue(c *tc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	rootDisk := uint64(1000)
	startInstArgs.Constraints.RootDisk = &rootDisk
	res, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*res.Hardware.RootDisk, tc.Equals, common.MinRootDiskSizeGiB(ostype.Ubuntu)*uint64(1024))
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceSelectZone(c *tc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	startInstArgs.AvailabilityZone = "z2"
	_, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorIsNil)

	s.client.CheckCallNames(c, "Folders", "ComputeResources", "ResourcePools", "ResourcePools", "GetTargetDatastore", "ListVMTemplates", "EnsureVMFolder", "CreateTemplateVM", "CreateVirtualMachine", "Close")
	call := s.client.Calls()[8]
	c.Assert(call.Args, tc.HasLen, 2)
	c.Assert(call.Args[0], tc.Implements, new(context.Context))
	c.Assert(call.Args[1], tc.FitsTypeOf, vsphereclient.CreateVirtualMachineParams{})

	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.ComputeResource, tc.DeepEquals, s.client.computeResources[1].Resource)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceFailsWithAvailabilityZone(c *tc.C) {
	s.client.SetErrors(nil, nil, nil, nil, errors.New("nope"))
	startInstArgs := s.createStartInstanceArgs(c)
	_, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.Not(tc.ErrorIs), environs.ErrAvailabilityZoneIndependent)

	s.client.CheckCallNames(c, "Folders", "ComputeResources", "ResourcePools", "ResourcePools", "GetTargetDatastore", "Close")
	getDatastoreCall := s.client.Calls()[4]
	getDataStoreArgs1 := getDatastoreCall.Args[1].(*mo.ComputeResource)
	c.Assert(getDataStoreArgs1, tc.DeepEquals, s.client.computeResources[0].Resource)
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceDatastoreDefault(c *tc.C) {
	cfg := s.env.Config()
	cfg, err := cfg.Apply(map[string]interface{}{
		"datastore": "datastore0",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.env.StartInstance(c.Context(), s.createStartInstanceArgs(c))
	c.Assert(err, tc.ErrorIsNil)

	call := s.client.Calls()[4]
	createVMArgs := call.Args[2].(string)
	c.Assert(createVMArgs, tc.Equals, "datastore0")
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceRootDiskSource(c *tc.C) {
	cfg := s.env.Config()
	cfg, err := cfg.Apply(map[string]interface{}{
		"datastore": "datastore0",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	args := s.createStartInstanceArgs(c)
	datastore := "zebras"
	args.Constraints.RootDiskSource = &datastore
	result, err := s.env.StartInstance(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*result.Hardware.RootDiskSource, tc.Equals, "zebras")

	call := s.client.Calls()[4]
	requestedDatastore := call.Args[2].(string)
	c.Assert(requestedDatastore, tc.Equals, "zebras")
}

type environBrokerSuite struct {
	coretesting.BaseSuite

	mockClient *mocks.MockClient
	provider   environs.CloudEnvironProvider
	env        environs.Environ

	imageServerURL string
}

func TestEnvironBrokerSuite(t *testing.T) {
	tc.Run(t, &environBrokerSuite{})
}

func (s *environBrokerSuite) setUpClient(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockClient = mocks.NewMockClient(ctrl)
	s.provider = vsphere.NewEnvironProvider(vsphere.EnvironProviderConfig{
		Dial: func(_ context.Context, _ *url.URL, _ string) (vsphere.Client, error) {
			return s.mockClient, nil
		},
	})
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"image-metadata-url": s.imageServerURL,
		}),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	s.env = env
	return ctrl
}

func (s *environBrokerSuite) TestStopInstances(c *tc.C) {
	ctrl := s.setUpClient(c)
	defer ctrl.Finish()

	s.mockClient.EXPECT().
		RemoveVirtualMachines(gomock.Any(), `Juju Controller (*)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)/vm-0`).
		Return(nil)
	s.mockClient.EXPECT().
		RemoveVirtualMachines(gomock.Any(), `Juju Controller (*)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)/vm-1`).
		Return(nil)
	s.mockClient.EXPECT().Close(gomock.Any()).Return(nil)

	err := s.env.StopInstances(c.Context(), "vm-0", "vm-1")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environBrokerSuite) TestStopInstancesOneFailure(c *tc.C) {
	ctrl := s.setUpClient(c)
	defer ctrl.Finish()

	failedRemoveVM := s.mockClient.EXPECT().
		RemoveVirtualMachines(gomock.Any(), `Juju Controller (*)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)/vm-0`).
		Return(errors.New("bah"))
	s.mockClient.EXPECT().
		RemoveVirtualMachines(gomock.Any(), `Juju Controller (*)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)/vm-1`).
		Return(nil)
	s.mockClient.EXPECT().
		FindFolder(gomock.Any(), "").
		After(failedRemoveVM.Call).Return(nil, nil) // only do find folder check if the RemoveVirtualMachines failed.
	s.mockClient.EXPECT().Close(gomock.Any()).Return(nil)

	err := s.env.StopInstances(c.Context(), "vm-0", "vm-1")
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("failed to stop instance %s: bah", "vm-0"))
}

func (s *environBrokerSuite) TestStopInstancesMultipleFailures(c *tc.C) {
	ctrl := s.setUpClient(c)
	defer ctrl.Finish()

	err1 := errors.New("bah")
	err2 := errors.New("bleh")

	failedRemoveVM1 := s.mockClient.EXPECT().
		RemoveVirtualMachines(gomock.Any(), `Juju Controller (*)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)/vm-0`).
		Return(err1)
	failedRemoveVM2 := s.mockClient.EXPECT().
		RemoveVirtualMachines(gomock.Any(), `Juju Controller (*)/Model "testmodel" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)/vm-1`).
		Return(err2)
	s.mockClient.EXPECT().
		FindFolder(gomock.Any(), "").
		After(failedRemoveVM1.Call).Return(nil, nil)
	s.mockClient.EXPECT().
		FindFolder(gomock.Any(), "").
		After(failedRemoveVM2.Call).Return(nil, nil)
	s.mockClient.EXPECT().Close(gomock.Any()).Return(nil)

	err := s.env.StopInstances(c.Context(), "vm-0", "vm-1")
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(
		`failed to stop instances \[vm-0 vm-1\]: \[%s %s\]`,
		err1, err2,
	))
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceLoginErrorInvalidatesCreds(c *tc.C) {
	s.dialStub.SetErrors(soap.WrapSoapFault(&soap.Fault{
		Code:   "ServerFaultCode",
		String: "You passed an incorrect user name or password, bucko.",
	}))
	_, err := s.env.StartInstance(c.Context(), s.createStartInstanceArgs(c))
	c.Assert(err, tc.ErrorMatches, "dialing client: ServerFaultCode: You passed an incorrect user name or password, bucko.")
	c.Assert(s.client.invalidReason, tc.Equals, "cloud denied access: ServerFaultCode: You passed an incorrect user name or password, bucko.")
}

func (s *legacyEnvironBrokerSuite) TestStartInstancePermissionError(c *tc.C) {
	AssertInvalidatesCredential(c, s.client, func(ctx context.Context) error {
		_, err := s.env.StartInstance(ctx, s.createStartInstanceArgs(c))
		return err
	})
}

func (s *legacyEnvironBrokerSuite) TestStopInstancesPermissionError(c *tc.C) {
	AssertInvalidatesCredential(c, s.client, func(ctx context.Context) error {
		return s.env.StopInstances(ctx, "vm-0")
	})
}

func (s *legacyEnvironBrokerSuite) TestStartInstanceNoDatastoreSetting(c *tc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	res, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorIsNil)

	s.client.CheckCallNames(c, "Folders", "ComputeResources", "ResourcePools", "ResourcePools", "GetTargetDatastore", "ListVMTemplates", "EnsureVMFolder", "CreateTemplateVM", "CreateVirtualMachine", "Close")
	call := s.client.Calls()[4]
	c.Assert(call.Args, tc.HasLen, 3)
	requestedDatastore := call.Args[2].(string)

	var expected string
	c.Assert(requestedDatastore, tc.Equals, expected)

	var (
		arch           = "amd64"
		rootDisk       = common.MinRootDiskSizeGiB(ostype.Ubuntu) * 1024
		rootDiskSource = ""
	)

	c.Assert(res.Hardware, tc.DeepEquals, &instance.HardwareCharacteristics{
		Arch:           &arch,
		RootDisk:       &rootDisk,
		RootDiskSource: &rootDiskSource,
	})
}

func (s *legacyEnvironBrokerSuite) TestNotBootstrapping(c *tc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	nonBootstrapInstance, err := instancecfg.NewInstanceConfig(
		names.NewControllerTag(coretesting.FakeControllerConfig().ControllerUUID()),
		"0",
		"nonce",
		"",
		corebase.MakeDefaultBase("ubuntu", "22.04"),
		&api.Info{
			Tag:      names.NewMachineTag("0"),
			ModelTag: coretesting.ModelTag,
			CACert:   "supersecret",
			Addrs:    []string{"hey:123"},
			Password: "mypassword1!",
		},
	)

	c.Assert(err, tc.ErrorIsNil)
	startInstArgs.InstanceConfig = nonBootstrapInstance

	result, err := s.env.StartInstance(c.Context(), startInstArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)
	c.Assert(result.Instance, tc.NotNil)
	c.Assert(result.Instance.Id(), tc.Equals, instance.Id("new-vm"))

	s.client.CheckCallNames(c, "Folders", "ComputeResources", "ResourcePools", "ResourcePools", "GetTargetDatastore", "ListVMTemplates", "EnsureVMFolder", "CreateTemplateVM", "CreateVirtualMachine", "Close")
	call := s.client.Calls()[8]
	c.Assert(call.Args, tc.HasLen, 2)
	c.Assert(call.Args[0], tc.Implements, new(context.Context))
	c.Assert(call.Args[1], tc.FitsTypeOf, vsphereclient.CreateVirtualMachineParams{})
}
