// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/provider/vsphere/internal/ovatest"
	coretesting "github.com/juju/juju/testing"
)

func (s *clientSuite) TestCreateVirtualMachine(c *gc.C) {
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

	args := baseCreateVirtualMachineParams(c)
	testClock := args.Clock.(*testclock.Clock)
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
	args.UpdateProgress = func(status string) {
		statusUpdatesCh <- status
		statusUpdates = append(statusUpdates, status)
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusUpdates, jc.DeepEquals, []string{
		fmt.Sprintf(`creating template VM "juju-template-%s"`, args.OVASHA256),
		"streaming vmdk: 100.00% (0B/s)",
		"cloning template",
		"powering on",
	})

	c.Assert(s.uploadRequests, gc.HasLen, 1)
	contents, err := ioutil.ReadAll(s.uploadRequests[0].Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, "FakeVmdkContent")

	templateCisp := baseCisp()
	templateCisp.EntityName = vmTemplateName(args)
	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeHostFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeDatastore1", "FakeDatastore2"),
		{"CreateImportSpec", []interface{}{
			UbuntuOVF,
			types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore2"},
			templateCisp,
		}},
		retrievePropertiesStubCall("network-0", "network-1"),
		retrievePropertiesStubCall("onetwork-0"), // "opaque network"
		retrievePropertiesStubCall("dvportgroup-0"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		{"CreateFolder", []interface{}{"juju-vmdks"}},
		{"CreateFolder", []interface{}{"ctrl"}},
		{"CreateFolder", []interface{}{"xenial"}},
		{"ImportVApp", []interface{}{
			&types.VirtualMachineImportSpec{
				ConfigSpec: types.VirtualMachineConfigSpec{
					Name: "vm-name",
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{Key: "k", Value: "v"},
					},
					Flags: &types.VirtualMachineFlagInfo{
						DiskUuidEnabled: newBool(true),
					},
				},
			},
		}},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
		{"HttpNfcLeaseComplete", []interface{}{"FakeLease"}},
		{"MarkAsTemplateBody", []interface{}{"FakeVm0"}},
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("network-0", "network-1"),
		retrievePropertiesStubCall("onetwork-0"),
		retrievePropertiesStubCall("dvportgroup-0"),
		retrievePropertiesStubCall("FakeVm0"),
		{"CloneVM_Task", []interface{}{
			&types.VmConfigSpec{
				Property: []types.VAppPropertySpec{{
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 1, Value: "vm-0"},
				}, {
					ArrayUpdateSpec: types.ArrayUpdateSpec{Operation: "edit"},
					Info:            &types.VAppPropertyInfo{Key: 4, Value: "baz"},
				}},
			},
		}},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
		{"PowerOnVM_Task", nil},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
		retrievePropertiesStubCall(""),
	})
}

func (s *clientSuite) TestCreateVirtualMachineNoDiskUUID(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.EnableDiskUUID = false
	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCall(c, 24, "ImportVApp", &types.VirtualMachineImportSpec{
		ConfigSpec: types.VirtualMachineConfigSpec{
			Name: "vm-name",
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "k", Value: "v"},
			},
			Flags: &types.VirtualMachineFlagInfo{DiskUuidEnabled: newBool(args.EnableDiskUUID)},
		},
	})
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreSpecified(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	datastore := "datastore1"
	args.Constraints.RootDiskSource = &datastore
	args.ComputeResource.Datastore = []types.ManagedObjectReference{{
		Type:  "Datastore",
		Value: "FakeDatastore2",
	}, {
		Type:  "Datastore",
		Value: "FakeDatastore1",
	}}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	//findStubCall(c, s.roundTripper.Calls(), "?")

	cisp := baseCisp()
	cisp.EntityName = vmTemplateName(args)
	s.roundTripper.CheckCall(
		c, 14, "CreateImportSpec", UbuntuOVF,
		types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"},
		cisp,
	)
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreNotFound(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	datastore := "datastore3"
	args.Constraints.RootDiskSource = &datastore

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `creating template VM: could not find datastore "datastore3", datastore\(s\) accessible: "datastore2"`)
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreNoneAccessible(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.ComputeResource.Datastore = []types.ManagedObjectReference{{
		Type:  "Datastore",
		Value: "FakeDatastore1",
	}}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "creating template VM: no accessible datastores available")
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreNotFoundWithMultipleAvailable(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	datastore := "datastore3"
	args.Constraints.RootDiskSource = &datastore

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

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `creating template VM: could not find datastore "datastore3", datastore\(s\) accessible: "datastore1", "datastore2"`)
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreNotFoundWithNoAvailable(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	datastore := "datastore3"
	args.Constraints.RootDiskSource = &datastore

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

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `creating template VM: no accessible datastores available`)
}

func (s *clientSuite) TestCreateVirtualMachineMultipleNetworksSpecifiedFirstDefault(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.NetworkDevices = []NetworkDevice{
		{MAC: "00:50:56:11:22:33"},
		{Network: "arpa"},
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	var networkDevice1, networkDevice2 types.VirtualVmxnet3
	wakeOnLan := true
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

	s.roundTripper.CheckCall(c, 24, "ImportVApp", &types.VirtualMachineImportSpec{
		ConfigSpec: types.VirtualMachineConfigSpec{
			Name: "vm-name",
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
		},
	})
}

func (s *clientSuite) TestCreateVirtualMachineNetworkSpecifiedDVPortgroup(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.NetworkDevices = []NetworkDevice{
		{Network: "yoink"},
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	var networkDevice types.VirtualVmxnet3
	wakeOnLan := true
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
	s.roundTripper.CheckCall(c, 18, retrieveDVSCall.FuncName, retrieveDVSCall.Args...)
	s.roundTripper.CheckCall(c, 36, retrieveDVSCall.FuncName, retrieveDVSCall.Args...)

	// When the external network is a distributed virtual portgroup,
	// we must make an additional RetrieveProperties call to fetch
	// the DVS's UUID. This bumps the ImportVApp position by one.
	s.roundTripper.CheckCall(c, 25, "ImportVApp", &types.VirtualMachineImportSpec{
		ConfigSpec: types.VirtualMachineConfigSpec{
			Name: "vm-name",
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
		},
	})
}

func (s *clientSuite) TestCreateVirtualMachineNetworkNotFound(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.NetworkDevices = []NetworkDevice{
		{Network: "fourtytwo"},
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `creating template VM: creating import spec: network "fourtytwo" not found`)
}

func (s *clientSuite) TestCreateVirtualMachineInvalidMAC(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.NetworkDevices = []NetworkDevice{
		{MAC: "00:11:22:33:44:55"},
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `creating template VM: creating import spec: adding network device 0 - network VM Network: Invalid MAC address: "00:11:22:33:44:55"`)
}

func (s *clientSuite) TestCreateVirtualMachineRootDiskSize(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	rootDisk := uint64(1024 * 20) // 20 GiB
	args.Constraints.RootDisk = &rootDisk

	client := s.newFakeClient(&s.roundTripper, "dc0")
	errCh := make(chan error)
	go func() {
		_, err := client.CreateVirtualMachine(context.Background(), args)
		select {
		case errCh <- err:
		case <-time.After(coretesting.ShortWait):
			c.Fatalf("timed out sending error back")
		}
	}()

	select {
	case <-errCh:
		c.Fatalf("creating virtual machine finished too soon")
	case <-time.After(coretesting.ShortWait):
	}

	err := s.clock.WaitAdvance(50*time.Millisecond, coretesting.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	// Report that the disk is big now.
	s.roundTripper.updateContents("FakeVm1", []types.ObjectContent{{
		Obj: types.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: "FakeVm1",
		},
		PropSet: []types.DynamicProperty{
			{Name: "name", Val: "vm-1"},
			{Name: "runtime.powerState", Val: "poweredOn"},
			{
				Name: "config.hardware.device",
				Val: []types.BaseVirtualDevice{
					&types.VirtualDisk{
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
		},
	}})

	select {
	case err := <-errCh:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for CreateVirtualMachine")
	}

	call := findStubCall(c, s.roundTripper.Calls(), "ExtendVirtualDisk")
	c.Assert(call.Args, jc.DeepEquals, []interface{}{
		"disk.vmdk",
		int64(rootDisk) * 1024, // in KiB
	})
}

func (s *clientSuite) TestCreateVirtualMachineTimesOut(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	rootDisk := uint64(1024 * 20) // 20 GiB
	args.Constraints.RootDisk = &rootDisk

	client := s.newFakeClient(&s.roundTripper, "dc0")
	errCh := make(chan error)
	go func() {
		_, err := client.CreateVirtualMachine(context.Background(), args)
		select {
		case errCh <- err:
		case <-time.After(coretesting.ShortWait):
			c.Fatalf("timed out sending error back")
		}
	}()

	select {
	case <-errCh:
		c.Fatalf("creating virtual machine finished too soon")
	case <-time.After(coretesting.ShortWait):
	}

	err := s.clock.WaitAdvance(35*time.Second, coretesting.LongWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-errCh:
		c.Assert(err, gc.ErrorMatches, "extending disk failed")
		c.Assert(err, jc.Satisfies, IsExtendDiskError)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for CreateVirtualMachine")
	}
}

func (s *clientSuite) TestVerifyMAC(c *gc.C) {
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
		c.Check(VerifyMAC(test.Mac), gc.Equals, test.Result)
	}
}

func baseCreateVirtualMachineParams(c *gc.C) CreateVirtualMachineParams {
	readOVA := func() (string, io.ReadCloser, error) {
		r := bytes.NewReader(ovatest.FakeOVAContents())
		return "fake-ova-location", ioutil.NopCloser(r), nil
	}

	return CreateVirtualMachineParams{
		Name:          "vm-0",
		Folder:        "foo",
		ReadOVA:       readOVA,
		OVASHA256:     ovatest.FakeOVASHA256(),
		VMDKDirectory: "juju-vmdks/ctrl",
		Series:        "xenial",
		UserData:      "baz",
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
		Metadata:               map[string]string{"k": "v"},
		Constraints:            constraints.Value{},
		UpdateProgress:         func(status string) {},
		UpdateProgressInterval: time.Second,
		Clock:                  testclock.NewClock(time.Time{}),
		EnableDiskUUID:         true,
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

func findStubCall(c *gc.C, calls []testing.StubCall, name string) testing.StubCall {
	c.Logf("finding stub \"%s\"", name)
	for i, call := range calls {
		c.Logf("stub %d: %s(%v)", i, call.FuncName, call.Args)
		if call.FuncName == name {
			return call
		}
	}
	c.Fatalf("failed to find call %q", name)
	panic("unreachable")
}

func assertNoCall(c *gc.C, calls []testing.StubCall, name string) {
	for _, call := range calls {
		if call.FuncName == name {
			c.Fatalf("found call %q", name)
		}
	}
}
