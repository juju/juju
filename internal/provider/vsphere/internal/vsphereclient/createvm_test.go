// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/provider/vsphere/internal/ovatest"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

func (s *clientSuite) TestCreateTemplateVM(c *tc.C) {
	var statusUpdates []string
	statusUpdatesCh := make(chan string, 4)
	dequeueStatusUpdates := func() {
		for {
			select {
			case <-statusUpdatesCh:
			default:
				return
			}
		}
	}
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseImportOVAParameters(c, client)
	testClock := args.StatusUpdateParams.Clock.(*testclock.Clock)
	s.onImageUpload = func(r *http.Request) {
		dequeueStatusUpdates()

		// Wait 1.5 seconds, which is long enough to trigger the
		// status update timer.
		testClock.WaitAdvance(1500*time.Millisecond, coretesting.LongWait, 1)

		// Waiting for the status update here guarantees that a report is
		// available, since we don't update status until that is true.
		<-statusUpdatesCh

		s.onImageUpload = nil
	}
	args.StatusUpdateParams.UpdateProgress = func(status string) {
		statusUpdatesCh <- status
		statusUpdates = append(statusUpdates, status)
	}

	_, err := client.CreateTemplateVM(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusUpdates, tc.DeepEquals, []string{
		fmt.Sprintf(`creating template VM "juju-template-%s"`, args.OVASHA256),
		"streaming vmdk: 100.00% (0B/s)",
	})
	c.Assert(s.uploadRequests, tc.HasLen, 1)
	contents, err := io.ReadAll(s.uploadRequests[0].Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(contents), tc.Equals, "FakeVmdkContent")

	templateCisp := baseCisp()
	templateCisp.EntityName = args.TemplateName
	s.roundTripper.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "CreateImportSpec", Args: []interface{}{
			UbuntuOVF,
			types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"},
			templateCisp,
		}},
		{FuncName: "ImportVApp", Args: []interface{}{
			&types.VirtualMachineImportSpec{
				ConfigSpec: types.VirtualMachineConfigSpec{
					Name: "vm-name",
				},
			},
		}},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
		{FuncName: "HttpNfcLeaseComplete", Args: []interface{}{"FakeLease"}},
		{FuncName: "ReconfigVM_Task", Args: []interface{}{
			types.VirtualMachineConfigSpec{
				ExtraConfig: []types.BaseOptionValue{
					&types.OptionValue{Key: ArchTag, Value: "amd64"},
				},
			},
		}},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
		{FuncName: "MarkAsTemplate", Args: []interface{}{"FakeVm0"}},
	})
}

func (s *clientSuite) TestCreateVirtualMachine(c *tc.C) {
	var statusUpdates []string
	statusUpdatesCh := make(chan string, 4)
	dequeueStatusUpdates := func() {
		for {
			select {
			case <-statusUpdatesCh:
			default:
				return
			}
		}
	}
	client := s.newFakeClient(c, &s.roundTripper, "dc0")

	args := baseCreateVirtualMachineParams(c, client)
	testClock := args.StatusUpdateParams.Clock.(*testclock.Clock)
	s.onImageUpload = func(r *http.Request) {
		dequeueStatusUpdates()

		// Wait 1.5 seconds, which is long enough to trigger the
		// status update timer.
		testClock.WaitAdvance(1500*time.Millisecond, coretesting.LongWait, 1)

		// Waiting for the status update here guarantees that a report is
		// available, since we don't update status until that is true.
		<-statusUpdatesCh

		s.onImageUpload = nil
	}
	args.StatusUpdateParams.UpdateProgress = func(status string) {
		statusUpdatesCh <- status
		statusUpdates = append(statusUpdates, status)
	}

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusUpdates, tc.DeepEquals, []string{
		"cloning template",
		"VM cloned",
		"powering on",
	})

	datastore := types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"}
	s.roundTripper.CheckCalls(c, []testhelpers.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeHostFolder"),
		retrievePropertiesStubCall("network-0", "network-1"),
		retrievePropertiesStubCall("onetwork-0"),
		retrievePropertiesStubCall("dvportgroup-0"),
		retrievePropertiesStubCall("FakeVm0"),
		retrievePropertiesStubCall("FakeVm0"),
		{FuncName: "CloneVM_Task", Args: []interface{}{
			types.ManagedObjectReference{
				Type: "Folder", Value: "FakeControllerVmFolder",
			},
			"vm-0",
			&types.VirtualMachineConfigSpec{
				ExtraConfig: []types.BaseOptionValue{
					&types.OptionValue{Key: "k", Value: "v"},
				},
				Flags: &types.VirtualMachineFlagInfo{
					DiskUuidEnabled: newBool(true),
				},
				VAppConfig: &types.VmConfigSpec{
					Property: []types.VAppPropertySpec{{
						ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
						Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
					}, {
						ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
						Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
					}},
				},
			},
			types.VirtualMachineRelocateSpec{
				Pool:      &args.ResourcePool,
				Datastore: &datastore,
				Disk: []types.VirtualMachineRelocateSpecDiskLocator{
					{
						DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
							EagerlyScrub:    newBool(false),
							ThinProvisioned: newBool(true),
						},
						DiskId:    0,
						Datastore: datastore,
					},
				},
			},
		}},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
		{FuncName: "PowerOnVM_Task", Args: nil},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
		retrievePropertiesStubCall(""),
	})
}

func (s *clientSuite) TestCreateVirtualMachineForceHWVersion(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.ForceVMHardwareVersion = 11
	args.ComputeResource.EnvironmentBrowser = &types.ManagedObjectReference{
		Type:  "EnvironmentBrowser",
		Value: "FakeEnvironmentBrowser",
	}
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	s.roundTripper.CheckCall(c, 18, "RetrieveProperties", "FakeVm1")
	s.roundTripper.CheckCall(c, 19, "QueryConfigOption", "FakeEnvironmentBrowser")
	// Mock server max version is vmx-13
	// Mock template VM version is vmx-10
	// We requested vmx-11. This should match the call to UpgradeVM_Task.
	s.roundTripper.CheckCall(c, 20, "UpgradeVM_Task", "vmx-11")
}

func (s *clientSuite) TestCreateVirtualMachineNoDiskUUID(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.EnableDiskUUID = false
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	datastore := types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"}
	s.roundTripper.CheckCall(
		c, 14, "CloneVM_Task",
		types.ManagedObjectReference{
			Type: "Folder", Value: "FakeControllerVmFolder",
		},
		"vm-0", &types.VirtualMachineConfigSpec{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "k", Value: "v"},
			},
			Flags: &types.VirtualMachineFlagInfo{DiskUuidEnabled: newBool(args.EnableDiskUUID)},
			VAppConfig: &types.VmConfigSpec{
				Property: []types.VAppPropertySpec{{
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
				}, {
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
				}},
			},
		}, types.VirtualMachineRelocateSpec{
			Pool:      &args.ResourcePool,
			Datastore: &datastore,
			Disk: []types.VirtualMachineRelocateSpecDiskLocator{
				{
					DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
						EagerlyScrub:    newBool(false),
						ThinProvisioned: newBool(true),
					},
					DiskId:    0,
					Datastore: datastore,
				},
			},
		})
}

func (s *clientSuite) TestCreateVirtualMachineThickDiskProvisioning(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.DiskProvisioningType = DiskTypeThickLazyZero
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	datastore := types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"}

	s.roundTripper.CheckCall(
		c, 14, "CloneVM_Task",
		types.ManagedObjectReference{
			Type: "Folder", Value: "FakeControllerVmFolder",
		},
		"vm-0", &types.VirtualMachineConfigSpec{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "k", Value: "v"},
			},
			Flags: &types.VirtualMachineFlagInfo{DiskUuidEnabled: newBool(true)},
			VAppConfig: &types.VmConfigSpec{
				Property: []types.VAppPropertySpec{{
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
				}, {
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
				}},
			},
		}, types.VirtualMachineRelocateSpec{
			Pool:      &args.ResourcePool,
			Datastore: &datastore,
			Disk: []types.VirtualMachineRelocateSpecDiskLocator{
				{
					DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
						// Thick disk provisioning, lazy zeros
						EagerlyScrub:    newBool(false),
						ThinProvisioned: newBool(false),
					},
					DiskId:    0,
					Datastore: datastore,
				},
			},
		})
}

func (s *clientSuite) TestCreateVirtualMachineThickEagerZeroDiskProvisioning(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.DiskProvisioningType = DiskTypeThick

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	datastore := types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"}

	s.roundTripper.CheckCall(
		c, 14, "CloneVM_Task",
		types.ManagedObjectReference{
			Type: "Folder", Value: "FakeControllerVmFolder",
		},
		"vm-0", &types.VirtualMachineConfigSpec{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "k", Value: "v"},
			},
			Flags: &types.VirtualMachineFlagInfo{DiskUuidEnabled: newBool(true)},
			VAppConfig: &types.VmConfigSpec{
				Property: []types.VAppPropertySpec{{
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
				}, {
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
				}},
			},
		}, types.VirtualMachineRelocateSpec{
			Pool:      &args.ResourcePool,
			Datastore: &datastore,
			Disk: []types.VirtualMachineRelocateSpecDiskLocator{
				{
					DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
						// Thick disk provisioning, eager zeros
						EagerlyScrub:    newBool(true),
						ThinProvisioned: newBool(false),
					},
					DiskId:    0,
					Datastore: datastore,
				},
			},
		})
}

func (s *clientSuite) TestCreateVirtualMachineThinDiskProvisioning(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.DiskProvisioningType = DiskTypeThin

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	datastore := types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"}

	s.roundTripper.CheckCall(c, 14, "CloneVM_Task", types.ManagedObjectReference{Type: "Folder", Value: "FakeControllerVmFolder"}, "vm-0", &types.VirtualMachineConfigSpec{
		ExtraConfig: []types.BaseOptionValue{
			&types.OptionValue{Key: "k", Value: "v"},
		},
		Flags: &types.VirtualMachineFlagInfo{DiskUuidEnabled: newBool(true)},
		VAppConfig: &types.VmConfigSpec{
			Property: []types.VAppPropertySpec{{
				ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
				Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
			}, {
				ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
				Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
			}},
		},
	}, types.VirtualMachineRelocateSpec{
		Pool:      &args.ResourcePool,
		Datastore: &datastore,
		Disk: []types.VirtualMachineRelocateSpecDiskLocator{
			{
				DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
					// Thin disk provisioning
					EagerlyScrub:    newBool(false),
					ThinProvisioned: newBool(true),
				},
				DiskId:    0,
				Datastore: datastore,
			},
		},
	})
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreSpecified(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	datastore := "datastore1"
	args.Constraints.RootDiskSource = &datastore
	args.ComputeResource.Datastore = []types.ManagedObjectReference{{
		Type:  "Datastore",
		Value: "FakeDatastore2",
	}, {
		Type:  "Datastore",
		Value: "FakeDatastore1",
	}}

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	datastoreLocation := types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"}
	s.roundTripper.CheckCall(
		c, 14, "CloneVM_Task", types.ManagedObjectReference{
			Type: "Folder", Value: "FakeControllerVmFolder",
		},
		"vm-0", &types.VirtualMachineConfigSpec{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "k", Value: "v"},
			},
			Flags: &types.VirtualMachineFlagInfo{DiskUuidEnabled: newBool(true)},
			VAppConfig: &types.VmConfigSpec{
				Property: []types.VAppPropertySpec{{
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
				}, {
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
				}},
			},
		}, types.VirtualMachineRelocateSpec{
			Pool:      &args.ResourcePool,
			Datastore: &datastoreLocation,
			Disk: []types.VirtualMachineRelocateSpecDiskLocator{
				{
					DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
						EagerlyScrub:    newBool(false),
						ThinProvisioned: newBool(true),
					},
					DiskId:    0,
					Datastore: datastoreLocation,
				},
			},
		})
}

func (s *clientSuite) TestGetTargetDatastoreDatastoreNotFound(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	datastore := "datastore3"

	_, err := client.GetTargetDatastore(context.Background(), args.ComputeResource, datastore)
	c.Assert(err, tc.ErrorMatches, `could not find datastore "datastore3", datastore\(s\) accessible: "datastore2"`)
}

func (s *clientSuite) TestGetTargetDatastoreDatastoreNoneAccessible(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.ComputeResource.Datastore = []types.ManagedObjectReference{{
		Type:  "Datastore",
		Value: "FakeDatastore1",
	}}

	_, err := client.GetTargetDatastore(context.Background(), args.ComputeResource, args.Datastore.Name())
	c.Assert(err, tc.ErrorMatches, "no accessible datastores available")
}

func (s *clientSuite) TestGetTargetDatastoreDatastoreNotFoundWithMultipleAvailable(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	datastore := "datastore3"

	s.roundTripper.updateContents("FakeDatastore1",
		[]types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "Datastore",
				Value: "FakeDatastore1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "datastore1"},
				{Name: "summary.accessible", Val: true},
			},
		}},
	)

	_, err := client.GetTargetDatastore(context.Background(), args.ComputeResource, datastore)
	c.Assert(err, tc.ErrorMatches, `could not find datastore "datastore3", datastore\(s\) accessible: "datastore1", "datastore2"`)
}

func (s *clientSuite) TestGetTargetDatastoreDatastoreNotFoundWithNoAvailable(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	datastore := "datastore3"

	s.roundTripper.updateContents("FakeDatastore2",
		[]types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "Datastore",
				Value: "FakeDatastore2",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "datastore2"},
				{Name: "summary.accessible", Val: false},
			},
		}},
	)

	_, err := client.GetTargetDatastore(context.Background(), args.ComputeResource, datastore)
	c.Assert(err, tc.ErrorMatches, `no accessible datastores available`)
}

func (s *clientSuite) TestCreateVirtualMachineMultipleNetworksSpecifiedFirstDefault(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.NetworkDevices = []NetworkDevice{
		{MAC: "00:50:56:11:22:33"},
		{Network: "arpa"},
	}

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	var networkDevice1, networkDevice2 types.VirtualVmxnet3
	wakeOnLan := true
	networkDevice1.Key = -1
	networkDevice1.WakeOnLanEnabled = &wakeOnLan
	networkDevice1.Connectable = &types.VirtualDeviceConnectInfo{
		StartConnected:    true,
		AllowGuestControl: true,
	}
	networkDevice1.AddressType = "Manual"
	networkDevice1.MacAddress = "00:50:56:11:22:33"
	networkDevice1.Backing = &types.VirtualEthernetCardNetworkBackingInfo{
		VirtualDeviceDeviceBackingInfo: types.VirtualDeviceDeviceBackingInfo{
			DeviceName: "VM Network",
		},
	}

	networkDevice2.Key = -2
	networkDevice2.WakeOnLanEnabled = &wakeOnLan
	networkDevice2.Connectable = &types.VirtualDeviceConnectInfo{
		StartConnected:    true,
		AllowGuestControl: true,
	}
	networkDevice2.Backing = &types.VirtualEthernetCardNetworkBackingInfo{
		VirtualDeviceDeviceBackingInfo: types.VirtualDeviceDeviceBackingInfo{
			DeviceName: "arpa",
		},
	}
	datastore := types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"}
	s.roundTripper.CheckCall(
		c, 14, "CloneVM_Task",
		types.ManagedObjectReference{
			Type: "Folder", Value: "FakeControllerVmFolder",
		},
		"vm-0", &types.VirtualMachineConfigSpec{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "k", Value: "v"},
			},
			DeviceChange: []types.BaseVirtualDeviceConfigSpec{
				&types.VirtualDeviceConfigSpec{
					Operation: "add",
					Device:    &networkDevice1,
				},
				&types.VirtualDeviceConfigSpec{
					Operation: "add",
					Device:    &networkDevice2,
				},
			},
			Flags: &types.VirtualMachineFlagInfo{DiskUuidEnabled: newBool(true)},
			VAppConfig: &types.VmConfigSpec{
				Property: []types.VAppPropertySpec{{
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
				}, {
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
				}},
			},
		}, types.VirtualMachineRelocateSpec{
			Pool:      &args.ResourcePool,
			Datastore: &datastore,
			Disk: []types.VirtualMachineRelocateSpecDiskLocator{
				{
					DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
						EagerlyScrub:    newBool(false),
						ThinProvisioned: newBool(true),
					},
					DiskId:    0,
					Datastore: datastore,
				},
			},
		})
}

func (s *clientSuite) TestCreateVirtualMachineNetworkSpecifiedDVPortgroup(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.NetworkDevices = []NetworkDevice{
		{Network: "yoink"},
	}

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	var networkDevice types.VirtualVmxnet3
	wakeOnLan := true
	networkDevice.Key = -1
	networkDevice.WakeOnLanEnabled = &wakeOnLan
	networkDevice.Connectable = &types.VirtualDeviceConnectInfo{
		StartConnected:    true,
		AllowGuestControl: true,
	}
	networkDevice.Backing = &types.VirtualEthernetCardDistributedVirtualPortBackingInfo{
		Port: types.DistributedVirtualSwitchPortConnection{
			SwitchUuid:   "yup",
			PortgroupKey: "hole",
		},
	}

	retrieveDVSCall := retrievePropertiesStubCall("dvs-0")
	s.roundTripper.CheckCall(c, 12, retrieveDVSCall.FuncName, retrieveDVSCall.Args...)

	datastore := types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"}
	// When the external network is a distributed virtual portgroup,
	// we must make an additional RetrieveProperties call to fetch
	// the DVS's UUID. This bumps the ImportVApp position by one.
	s.roundTripper.CheckCall(
		c, 15, "CloneVM_Task",
		types.ManagedObjectReference{
			Type: "Folder", Value: "FakeControllerVmFolder",
		},
		"vm-0", &types.VirtualMachineConfigSpec{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "k", Value: "v"},
			},
			DeviceChange: []types.BaseVirtualDeviceConfigSpec{
				&types.VirtualDeviceConfigSpec{
					Operation: "add",
					Device:    &networkDevice,
				},
			},
			Flags: &types.VirtualMachineFlagInfo{DiskUuidEnabled: newBool(true)},
			VAppConfig: &types.VmConfigSpec{
				Property: []types.VAppPropertySpec{{
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
				}, {
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
				}},
			},
		}, types.VirtualMachineRelocateSpec{
			Pool:      &args.ResourcePool,
			Datastore: &datastore,
			Disk: []types.VirtualMachineRelocateSpecDiskLocator{
				{
					DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
						EagerlyScrub:    newBool(false),
						ThinProvisioned: newBool(true),
					},
					DiskId:    0,
					Datastore: datastore,
				},
			},
		})
}

func (s *clientSuite) TestCreateVirtualMachineNetworkNotFound(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.NetworkDevices = []NetworkDevice{
		{Network: "fourtytwo"},
	}

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorMatches, `cloning template VM: building clone VM config: network "fourtytwo" not found`)
}

func (s *clientSuite) TestCreateVirtualMachineInvalidMAC(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	args.NetworkDevices = []NetworkDevice{
		{MAC: "00:11:22:33:44:55"},
	}

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorMatches, `cloning template VM: building clone VM config: adding network device 0 - network VM Network: invalid MAC address: "00:11:22:33:44:55"`)
}

func (s *clientSuite) TestCreateVirtualMachineRootDiskSize(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	rootDisk := uint64(1024 * 20) // 20 GiB
	args.Constraints.RootDisk = &rootDisk

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	s.roundTripper.CheckCall(c, 19, "ReconfigVM_Task", types.VirtualMachineConfigSpec{
		DeviceChange: []types.BaseVirtualDeviceConfigSpec{
			&types.VirtualDeviceConfigSpec{
				Operation:     types.VirtualDeviceConfigSpecOperationEdit,
				FileOperation: "",
				Device: &types.VirtualDisk{
					VirtualDevice: types.VirtualDevice{
						Backing: &types.VirtualDiskFlatVer2BackingInfo{
							VirtualDeviceFileBackingInfo: types.VirtualDeviceFileBackingInfo{
								FileName: "disk.vmdk",
							},
						},
					},
					CapacityInKB: 1024 * 1024 * 20, // 20 GiB
				},
			},
		},
	})
}

func (s *clientSuite) TestCreateVirtualMachineWithCustomizedVMFolder(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	args := baseCreateVirtualMachineParams(c, client)
	rootDisk := uint64(1024 * 20) // 20 GiB
	args.Constraints.RootDisk = &rootDisk

	args.Folder = "k8s"

	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	datastore := types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"}
	// The template import and the create from template have been split in two separate
	// functions. We now have to check the folder passed to CloneVm to determine if the
	// correct folder was selected when testing CreateVirtualMachine().
	s.roundTripper.CheckCall(
		c, 14, "CloneVM_Task",
		types.ManagedObjectReference{Type: "Folder", Value: "FakeK8sVMFolder"},
		"vm-0", &types.VirtualMachineConfigSpec{
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "k", Value: "v"},
			},
			Flags: &types.VirtualMachineFlagInfo{DiskUuidEnabled: newBool(args.EnableDiskUUID)},
			VAppConfig: &types.VmConfigSpec{
				Property: []types.VAppPropertySpec{{
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
				}, {
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
				}},
			},
		}, types.VirtualMachineRelocateSpec{
			Pool:      &args.ResourcePool,
			Datastore: &datastore,
			Disk: []types.VirtualMachineRelocateSpecDiskLocator{
				{
					DiskBackingInfo: &types.VirtualDiskFlatVer2BackingInfo{
						EagerlyScrub:    newBool(false),
						ThinProvisioned: newBool(true),
					},
					DiskId:    0,
					Datastore: datastore,
				},
			},
		})
}

func (s *clientSuite) TestVerifyMAC(c *tc.C) {
	var testData = []struct {
		Mac    string
		Result bool
	}{
		{"foo:bar:baz", false},
		{"00:22:55:11:34:11", false},
		{"00:50:56:123:11:11", false},
		{"00:50:56:40:12:23", false},
		{"00:50:56:3f:ff:ff", true},
		{"00:50:56:12:34:56", true},
		{"00:50:56:2A:eB:Cd", true},
		{"00:50:56:2a:xy:cd", false},
		{"00:50:560:2a:xy:cd", false},
	}
	for i, test := range testData {
		c.Logf("test #%d: MAC=%s expected %s", i, test.Mac, test.Result)
		c.Check(VerifyMAC(test.Mac), tc.Equals, test.Result)
	}
}

func baseImportOVAParameters(c *tc.C, client *Client) ImportOVAParameters {
	readOVA := func() (string, io.ReadCloser, error) {
		r := bytes.NewReader(ovatest.FakeOVAContents())
		return "fake-ova-location", io.NopCloser(r), nil
	}
	fakeSHA256 := ovatest.FakeOVASHA256()
	fakeDS := types.ManagedObjectReference{
		Type:  "Datastore",
		Value: "FakeDatastore1",
	}
	return ImportOVAParameters{
		ReadOVA:   readOVA,
		OVASHA256: fakeSHA256,
		StatusUpdateParams: StatusUpdateParams{
			UpdateProgress:         func(status string) {},
			UpdateProgressInterval: time.Second,
			Clock:                  testclock.NewClock(time.Time{}),
		},
		ResourcePool: types.ManagedObjectReference{
			Type:  "ResourcePool",
			Value: "FakeResourcePool1",
		},
		TemplateName: "juju-template-" + fakeSHA256,
		Arch:         "amd64",
		Base:         base.MustParseBaseFromString("ubuntu@16.04"),
		DestinationFolder: &object.Folder{
			Common: object.Common{
				InventoryPath: "/dc0/vm/juju-vmdks/ctrl/ubuntu_16.04",
			},
		},
		Datastore: object.NewDatastore(client.client.Client, fakeDS),
	}
}

func baseCreateVirtualMachineParams(c *tc.C, client *Client) CreateVirtualMachineParams {
	fakeVM := types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: "FakeVm0",
	}

	fakeDS := types.ManagedObjectReference{
		Type:  "Datastore",
		Value: "FakeDatastore1",
	}

	return CreateVirtualMachineParams{
		Name:     "vm-0",
		Folder:   "foo",
		UserData: "baz",
		ComputeResource: &mo.ComputeResource{
			ResourcePool: &types.ManagedObjectReference{
				Type:  "ResourcePool",
				Value: "FakeResourcePool1",
			},
			Datastore: []types.ManagedObjectReference{{
				Type:  "Datastore",
				Value: "FakeDatastore1",
			}, {
				Type:  "Datastore",
				Value: "FakeDatastore2",
			}},
			Network: []types.ManagedObjectReference{{
				Type:  "Network",
				Value: "network-0",
			}, {
				Type:  "Network",
				Value: "network-1",
			}, {
				Type:  "OpaqueNetwork",
				Value: "onetwork-0",
			}, {
				Type:  "DistributedVirtualPortgroup",
				Value: "dvportgroup-0",
			}},
		},
		ResourcePool: types.ManagedObjectReference{
			Type:  "ResourcePool",
			Value: "FakeResourcePool1",
		},
		Metadata:    map[string]string{"k": "v"},
		Constraints: constraints.Value{},
		StatusUpdateParams: StatusUpdateParams{
			UpdateProgress:         func(status string) {},
			UpdateProgressInterval: time.Second,
			Clock:                  testclock.NewClock(time.Time{}),
		},
		EnableDiskUUID:       true,
		DiskProvisioningType: DiskTypeThin,
		VMTemplate:           object.NewVirtualMachine(client.client.Client, fakeVM),
		Datastore:            object.NewDatastore(client.client.Client, fakeDS),
	}
}

func baseCisp() types.OvfCreateImportSpecParams {
	return types.OvfCreateImportSpecParams{
		EntityName: "vm-0",
	}
}

func newBool(v bool) *bool {
	return &v
}
