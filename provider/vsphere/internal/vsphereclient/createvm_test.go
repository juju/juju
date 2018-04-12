// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
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
	testClock := args.Clock.(*testing.Clock)
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
		"uploading juju-vmdks/ctrl/xenial/04d3415158bcc24a91334eda408cf102fcf45c56a805135493e00156cd7b6391.vmdk.tmp: 100.00% (0B/s)",
		"creating import spec",
		`creating VM "vm-0"`,
		"VM cloned",
		"powering on",
	})

	c.Assert(s.uploadRequests, gc.HasLen, 1)
	contents, err := ioutil.ReadAll(s.uploadRequests[0].Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, "FakeVmdkContent")

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeHostFolder"),
		retrievePropertiesStubCall("FakeDatastore1", "FakeDatastore2"),
		retrievePropertiesStubCall("FakeDatastore2"),

		testing.StubCall{"SearchDatastore", []interface{}{
			"[datastore2] juju-vmdks/ctrl/xenial",
			&types.HostDatastoreBrowserSearchSpec{
				MatchPattern: []string{"04d3415158bcc24a91334eda408cf102fcf45c56a805135493e00156cd7b6391.vmdk"},
				Details: &types.FileQueryFlags{
					FileType:     true,
					FileSize:     true,
					Modification: true,
					FileOwner:    newBool(true),
				},
			},
		}},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},

		testing.StubCall{"DeleteDatastoreFile", []interface{}{
			"[datastore2] juju-vmdks/ctrl/xenial",
		}},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},

		testing.StubCall{"MakeDirectory", []interface{}{
			"[datastore2] juju-vmdks/ctrl/xenial",
		}},

		testing.StubCall{"MoveDatastoreFile", []interface{}{
			"[datastore2] juju-vmdks/ctrl/xenial/04d3415158bcc24a91334eda408cf102fcf45c56a805135493e00156cd7b6391.vmdk.tmp",
			"[datastore2] juju-vmdks/ctrl/xenial/04d3415158bcc24a91334eda408cf102fcf45c56a805135493e00156cd7b6391.vmdk",
			newBool(true),
		}},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},

		testing.StubCall{"CreateImportSpec", []interface{}{
			UbuntuOVF,
			types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore2"},
			baseCisp(),
		}},
		retrievePropertiesStubCall("network-0", "network-1"),
		retrievePropertiesStubCall("onetwork-0"),
		retrievePropertiesStubCall("dvportgroup-0"),
		testing.StubCall{"ImportVApp", []interface{}{&types.VirtualMachineImportSpec{
			ConfigSpec: types.VirtualMachineConfigSpec{
				Name: "vm-name.tmp",
				ExtraConfig: []types.BaseOptionValue{
					&types.OptionValue{Key: "k", Value: "v"},
				},
			},
		}}},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},

		testing.StubCall{"HttpNfcLeaseComplete", []interface{}{"FakeLease"}},

		testing.StubCall{"CloneVM_Task", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},

		retrievePropertiesStubCall("FakeVm0"),

		testing.StubCall{"ReconfigVM_Task", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},

		testing.StubCall{"PowerOnVM_Task", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},

		retrievePropertiesStubCall(""),

		testing.StubCall{"Destroy_Task", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},
	})
}

func (s *clientSuite) TestCreateVirtualMachineVMDKDirectoryNotFound(c *gc.C) {
	// FileNotFound is returned when the *directory* doesn't exist.
	s.roundTripper.taskError[searchDatastoreTask] = &types.LocalizedMethodFault{
		Fault: &types.FileNotFound{},
	}

	args := baseCreateVirtualMachineParams(c)
	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	calls := s.roundTripper.Calls()
	assertNoCall(c, calls, "DeleteDatastoreFile")
	findStubCall(c, calls, "MakeDirectory")
}

func (s *clientSuite) TestCreateVirtualMachineDiskAlreadyCached(c *gc.C) {
	results := types.HostDatastoreBrowserSearchResults{
		File: []types.BaseFileInfo{&types.VmDiskFileInfo{}},
	}
	s.roundTripper.taskResult[searchDatastoreTask] = results

	args := baseCreateVirtualMachineParams(c)
	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	// There should be no upload, and the VMDK directory should neither
	// have been deleted nor created.
	calls := s.roundTripper.Calls()
	assertNoCall(c, calls, "DeleteDatastoreFile")
	assertNoCall(c, calls, "MakeDirectory")
	c.Assert(s.uploadRequests, gc.HasLen, 0)
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreSpecified(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.Datastore = "datastore1"
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

	s.roundTripper.CheckCall(
		c, 22, "CreateImportSpec", UbuntuOVF,
		types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"},
		baseCisp(),
	)
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreNotFound(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.Datastore = "datastore3"

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `could not find datastore "datastore3"`)
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreNoneAccessible(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.ComputeResource.Datastore = []types.ManagedObjectReference{{
		Type:  "Datastore",
		Value: "FakeDatastore1",
	}}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "could not find an accessible datastore")
}

func (s *clientSuite) TestCreateVirtualMachineMultipleNetworksSpecifiedFirstDefault(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.NetworkDevices = []NetworkDevice{
		NetworkDevice{MAC: "00:50:56:11:22:33"},
		NetworkDevice{Network: "arpa"},
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

	s.roundTripper.CheckCall(c, 26, "ImportVApp", &types.VirtualMachineImportSpec{
		ConfigSpec: types.VirtualMachineConfigSpec{
			Name: "vm-name.tmp",
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
		},
	})
}

func (s *clientSuite) TestCreateVirtualMachineNetworkSpecifiedDVPortgroup(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.NetworkDevices = []NetworkDevice{
		NetworkDevice{Network: "yoink"},
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
	s.roundTripper.CheckCall(c, 26, retrieveDVSCall.FuncName, retrieveDVSCall.Args...)

	// When the external network is a distributed virtual portgroup,
	// we must make an additional RetrieveProperties call to fetch
	// the DVS's UUID. This bumps the ImportVApp position by one.
	s.roundTripper.CheckCall(c, 27, "ImportVApp", &types.VirtualMachineImportSpec{
		ConfigSpec: types.VirtualMachineConfigSpec{
			Name: "vm-name.tmp",
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{Key: "k", Value: "v"},
			},
			DeviceChange: []types.BaseVirtualDeviceConfigSpec{
				&types.VirtualDeviceConfigSpec{
					Operation: "add",
					Device:    &networkDevice,
				},
			},
		},
	})
}

func (s *clientSuite) TestCreateVirtualMachineNetworkNotFound(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.NetworkDevices = []NetworkDevice{
		NetworkDevice{Network: "fourtytwo"},
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `creating import spec: network "fourtytwo" not found`)
}

func (s *clientSuite) TestCreateVirtualMachineInvalidMAC(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.NetworkDevices = []NetworkDevice{
		NetworkDevice{MAC: "00:11:22:33:44:55"},
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `creating import spec: adding network device 0 - network VM Network: Invalid MAC address: "00:11:22:33:44:55"`)
}

func (s *clientSuite) TestCreateVirtualMachineRootDiskSize(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	rootDisk := uint64(1024 * 20) // 20 GiB
	args.Constraints.RootDisk = &rootDisk

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	call := findStubCall(c, s.roundTripper.Calls(), "ExtendVirtualDisk")
	c.Assert(call.Args, jc.DeepEquals, []interface{}{
		"disk.vmdk",
		int64(rootDisk) * 1024, // in KiB
	})
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
		Metadata:               map[string]string{"k": "v"},
		Constraints:            constraints.Value{},
		UpdateProgress:         func(status string) {},
		UpdateProgressInterval: time.Second,
		Clock: testing.NewClock(time.Time{}),
	}
}

func baseCisp() types.OvfCreateImportSpecParams {
	return types.OvfCreateImportSpecParams{
		EntityName: "vm-0",
		PropertyMapping: []types.KeyValue{
			{Key: "user-data", Value: "baz"},
			{Key: "hostname", Value: "vm-0"},
		},
	}
}

func newBool(v bool) *bool {
	return &v
}

func findStubCall(c *gc.C, calls []testing.StubCall, name string) testing.StubCall {
	for _, call := range calls {
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
