// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/mutex"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
)

type clientSuite struct {
	testing.IsolationSuite

	server         *httptest.Server
	serviceContent types.ServiceContent
	roundTripper   mockRoundTripper
	uploadRequests []*http.Request
	onImageUpload  func(*http.Request)
	clock          *testclock.Clock
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.serviceContent = types.ServiceContent{
		RootFolder: types.ManagedObjectReference{
			Type:  "Folder",
			Value: "FakeRootFolder",
		},
		OvfManager: &types.ManagedObjectReference{
			Type:  "OvfManager",
			Value: "FakeOvfManager",
		},
		SessionManager: &types.ManagedObjectReference{
			Type:  "SessionManager",
			Value: "FakeSessionManager",
		},
		FileManager: &types.ManagedObjectReference{
			Type:  "FileManager",
			Value: "FakeFileManager",
		},
		VirtualDiskManager: &types.ManagedObjectReference{
			Type:  "VirtualDiskManager",
			Value: "FakeVirtualDiskManager",
		},
		PropertyCollector: types.ManagedObjectReference{
			Type:  "PropertyCollector",
			Value: "FakePropertyCollector",
		},
		SearchIndex: &types.ManagedObjectReference{
			Type:  "SearchIndex",
			Value: "FakeSearchIndex",
		},
		AuthorizationManager: &types.ManagedObjectReference{
			Type:  "AuthorizationManager",
			Value: "FakeAuthorizationManager",
		},
	}
	s.roundTripper = mockRoundTripper{
		collectors: make(map[string]*collector),
		taskResult: make(map[types.ManagedObjectReference]types.AnyType),
		taskError:  make(map[types.ManagedObjectReference]*types.LocalizedMethodFault),
	}
	s.roundTripper.setContents(map[string][]types.ObjectContent{
		"FakeRootFolder": {{
			Obj: types.ManagedObjectReference{
				Type:  "Datacenter",
				Value: "FakeDatacenter",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "dc0"},
			},
		}},
		"FakeSessionManager": {{
			Obj: types.ManagedObjectReference{
				Type:  "SessionManager",
				Value: "FakeSessionManager",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "sm"},
				{
					Name: "currentSession",
					Val: types.UserSession{
						Key: "session-key",
					},
				},
			},
		}},
		"FakeDatacenter": {{
			Obj: types.ManagedObjectReference{
				Type:  "Datacenter",
				Value: "FakeDatacenter",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "dc0"},
				{Name: "hostFolder", Val: types.ManagedObjectReference{
					Type:  "Folder",
					Value: "FakeHostFolder",
				}},
				{Name: "vmFolder", Val: types.ManagedObjectReference{
					Type:  "Folder",
					Value: "FakeVmFolder",
				}},
				{Name: "datastoreFolder", Val: types.ManagedObjectReference{
					Type:  "Folder",
					Value: "FakeDatastoreFolder",
				}},
			},
		}, {
			Obj: types.ManagedObjectReference{
				Type:  "Folder",
				Value: "FakeVmFolder",
			},
			PropSet: []types.DynamicProperty{{Name: "name", Val: "vm"}},
		}, {
			Obj: types.ManagedObjectReference{
				Type:  "Folder",
				Value: "FakeHostFolder",
			},
			PropSet: []types.DynamicProperty{{Name: "name", Val: "vm"}},
		}},
		"FakeHostFolder": {{
			Obj: types.ManagedObjectReference{
				Type:  "ComputeResource",
				Value: "z0",
			},
			PropSet: []types.DynamicProperty{
				{Name: "resourcePool", Val: types.ManagedObjectReference{
					Type:  "ResourcePool",
					Value: "FakeResourcePool1",
				}},
				{Name: "datastore", Val: []types.ManagedObjectReference{{
					Type:  "Datastore",
					Value: "FakeDatastore1",
				}}},
				{Name: "name", Val: "z0"},
			},
		}, {
			Obj: types.ManagedObjectReference{
				Type:  "ComputeResource",
				Value: "z1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "resourcePool", Val: types.ManagedObjectReference{
					Type:  "ResourcePool",
					Value: "FakeResourcePool2",
				}},
				{Name: "datastore", Val: []types.ManagedObjectReference{{
					Type:  "Datastore",
					Value: "FakeDatastore2",
				}}},
				{Name: "name", Val: "z1"},
			},
		}},
		"z0": {{
			Obj: types.ManagedObjectReference{
				Type:  "ResourcePool",
				Value: "FakeResourcePool1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "Resources"},
			},
		}},
		"FakeResourcePool1": {{
			Obj: types.ManagedObjectReference{
				Type:  "ResourcePool",
				Value: "FakeResourcePoolParent",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "parent"},
			},
		}, {
			Obj: types.ManagedObjectReference{
				Type:  "ResourcePool",
				Value: "FakeResourcePoolOther",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "other"},
			},
		}},
		"FakeResourcePoolParent": {{
			Obj: types.ManagedObjectReference{
				Type:  "ResourcePool",
				Value: "FakeResourcePoolChild",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "child"},
			},
		}},
		"FakeDatastoreFolder": {{
			Obj: types.ManagedObjectReference{
				Type:  "Datastore",
				Value: "FakeDatastore1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "datastore1"},
				{Name: "summary.accessible", Val: false},
			},
		}, {
			Obj: types.ManagedObjectReference{
				Type:  "Datastore",
				Value: "FakeDatastore2",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "datastore2"},
				{Name: "summary.accessible", Val: true},
			},
		}},
		"FakeVmFolder": {
			{
				Obj: types.ManagedObjectReference{
					Type:  "Folder",
					Value: "FakeControllerVmFolder",
				},
				PropSet: []types.DynamicProperty{
					{Name: "name", Val: "foo"},
				},
			},
			{
				Obj: types.ManagedObjectReference{
					Type:  "Folder",
					Value: "FakeK8sVMFolder",
				},
				PropSet: []types.DynamicProperty{
					{Name: "name", Val: "k8s"},
				},
			},
		},
		"FakeControllerVmFolder": {{
			Obj: types.ManagedObjectReference{
				Type:  "Folder",
				Value: "FakeModelVmFolder",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "bar"},
			},
		}},
		"FakeK8sVMFolder": {},
		"FakeModelVmFolder": {{
			Obj: types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: "FakeVmTemplate",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "juju-vm-template"},
			},
		},
			{
				Obj: types.ManagedObjectReference{
					Type:  "VirtualMachine",
					Value: "FakeVm0",
				},
				PropSet: []types.DynamicProperty{
					{Name: "name", Val: "juju-vm-0"},
				},
			}, {
				Obj: types.ManagedObjectReference{
					Type:  "VirtualMachine",
					Value: "FakeVm1",
				},
				PropSet: []types.DynamicProperty{
					{Name: "name", Val: "juju-vm-1"},
				},
			}},
		"FakeVm0": {{
			Obj: types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: "FakeVm0",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "juju-vm-0"},
				{Name: "runtime.powerState", Val: "poweredOff"},
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
						},
					},
				},
				{
					Name: "resourcePool",
					Val: types.ManagedObjectReference{
						Type:  "ResourcePool",
						Value: "FakeResourcePool0",
					},
				},
				{
					Name: "config.vAppConfig",
					Val: &types.VmConfigInfo{
						Product: []types.VAppProductInfo{
							{
								Key:  0,
								Name: "Ubuntu 16.04 Server (20170815)",
							},
						},
						Property: []types.VAppPropertyInfo{{
							Key:              0,
							Id:               "instance-id",
							Label:            "A Unique Instance ID for this instance",
							Type:             "string",
							UserConfigurable: types.NewBool(true),
							DefaultValue:     "id-ovf",
							Value:            "",
						}, {
							Key:              1,
							Id:               "hostname",
							Label:            "hostname",
							Type:             "string",
							UserConfigurable: types.NewBool(true),
							DefaultValue:     "ubuntuguest",
							Value:            "",
							Description:      "Specifies the hostname for the appliance",
						}, {
							Key:              2,
							Id:               "seedfrom",
							Label:            "Url to seed instance data from",
							Type:             "string",
							UserConfigurable: types.NewBool(true),
							DefaultValue:     "",
							Value:            "",
						}, {
							Key:              3,
							Id:               "public-keys",
							Label:            "ssh public keys",
							Type:             "string",
							UserConfigurable: types.NewBool(true),
							DefaultValue:     "",
							Value:            "",
						}, {
							Key:              4,
							Id:               "user-data",
							Label:            "Encoded user-data",
							Type:             "string",
							UserConfigurable: types.NewBool(true),
							DefaultValue:     "",
							Value:            "",
						}, {
							Key:              5,
							Id:               "password",
							Label:            "Default User's password",
							Type:             "string",
							UserConfigurable: types.NewBool(true),
							DefaultValue:     "",
							Value:            "",
						}},
					},
				},
			},
		}},
		"FakeVm1": {{
			Obj: types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: "FakeVm1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "juju-vm-1"},
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
							CapacityInKB: 1024 * 1024 * 10, // 10 GiB
						},
					},
				},
				{
					Name: "resourcePool",
					Val: types.ManagedObjectReference{
						Type:  "ResourcePool",
						Value: "FakeResourcePool1",
					},
				},
			},
		}},
		"FakeVmTemplate": {{
			Obj: types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: "FakeVmTemplate",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "juju-vm-template"},
				{Name: "runtime.powerState", Val: "poweredOff"},
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
							CapacityInKB: 1024 * 1024 * 10, // 10 GiB
						},
					},
				},
				{
					Name: "resourcePool",
					Val: types.ManagedObjectReference{
						Type:  "ResourcePool",
						Value: "FakeResourcePool1",
					},
				},
			},
		}},
		"FakeDatastore1": {{
			Obj: types.ManagedObjectReference{
				Type:  "Datastore",
				Value: "FakeDatastore1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "datastore1"},
				{Name: "summary.accessible", Val: false},
			},
		}},
		"FakeDatastore2": {{
			Obj: types.ManagedObjectReference{
				Type:  "Datastore",
				Value: "FakeDatastore2",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "datastore2"},
				{Name: "summary.accessible", Val: true},
			},
		}},
		"network-0": {{
			Obj: types.ManagedObjectReference{
				Type:  "Network",
				Value: "network-0",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "VM Network"},
			},
		}},
		"network-1": {{
			Obj: types.ManagedObjectReference{
				Type:  "Network",
				Value: "network-1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "zing"},
			},
		}},
		"onetwork-0": {{
			Obj: types.ManagedObjectReference{
				Type:  "OpaqueNetwork",
				Value: "onetwork-0",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "arpa"},
			},
		}},
		"dvportgroup-0": {{
			Obj: types.ManagedObjectReference{
				Type:  "DistributedVirtualPortgroup",
				Value: "dvportgroup-0",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "yoink"},
				{Name: "config.key", Val: "hole"},
				{
					Name: "config.distributedVirtualSwitch",
					Val: types.ManagedObjectReference{
						Type:  "DistributedVirtualSwitch",
						Value: "dvs-0",
					},
				},
			},
		}},
		"dvs-0": {{
			Obj: types.ManagedObjectReference{
				Type:  "DistributedVirtualSwitch",
				Value: "dvs-0",
			},
			PropSet: []types.DynamicProperty{
				{Name: "uuid", Val: "yup"},
			},
		}},
	})

	s.roundTripper.importVAppResult = types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: "FakeVm0",
	}
	s.roundTripper.taskResult[searchDatastoreTask] = types.HostDatastoreBrowserSearchResults{}
	s.roundTripper.taskResult[cloneVMTask] = types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: "FakeVm1",
	}

	// Create an HTTP server to receive image uploads.
	s.uploadRequests = nil
	s.onImageUpload = nil
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Infof("HTTP %#v", r)
		var buf bytes.Buffer
		io.Copy(&buf, r.Body)
		rcopy := *r
		rcopy.Body = ioutil.NopCloser(&buf)
		s.uploadRequests = append(s.uploadRequests, &rcopy)
		if s.onImageUpload != nil {
			s.onImageUpload(r)
		}
	}))
	s.AddCleanup(func(*gc.C) {
		s.server.Close()
	})
	s.roundTripper.serverURL = s.server.URL
	s.clock = testclock.NewClock(time.Now())
}

func (s *clientSuite) newFakeClient(roundTripper soap.RoundTripper, dc string) *Client {
	soapURL, err := url.Parse(s.server.URL + "/soap")
	if err != nil {
		panic(err)
	}

	vimClient := &vim25.Client{
		Client:         soap.NewClient(soapURL, true),
		ServiceContent: s.serviceContent,
		RoundTripper:   roundTripper,
	}
	return &Client{
		client: &govmomi.Client{
			Client:         vimClient,
			SessionManager: session.NewManager(vimClient),
		},
		datacenter:   dc,
		logger:       loggo.GetLogger("vsphereclient"),
		clock:        s.clock,
		acquireMutex: fakeAcquire,
	}
}

func (s *clientSuite) TestDial(c *gc.C) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		e := xml.NewEncoder(w)
		e.Encode(soap.Envelope{Body: methods.RetrieveServiceContentBody{
			Res: &types.RetrieveServiceContentResponse{s.serviceContent},
		}})
		e.Flush()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	url, err := url.Parse(server.URL)
	c.Assert(err, jc.ErrorIsNil)

	ctx := context.Background()
	client, err := Dial(ctx, url, "dc", loggo.GetLogger("vsphereclient"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(client, gc.NotNil)
}

func (s *clientSuite) TestClose(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.Close(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	s.roundTripper.CheckCallNames(c, "Logout")
}

func (s *clientSuite) TestComputeResources(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	result, err := client.ComputeResources(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeHostFolder"),
	})

	c.Assert(result, gc.HasLen, 2)
	c.Assert(result[0].Name, gc.Equals, "z0")
	c.Assert(result[1].Name, gc.Equals, "z1")
}

func (s *clientSuite) TestDestroyVMFolder(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.DestroyVMFolder(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeHostFolder"),
		{"Destroy_Task", nil},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
	})
}

func (s *clientSuite) TestDestroyVMFolderRace(c *gc.C) {
	s.roundTripper.taskError[destroyTask] = &types.LocalizedMethodFault{
		Fault: &types.ManagedObjectNotFound{},
	}
	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.DestroyVMFolder(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestEnsureVMFolder(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	folder, err := client.EnsureVMFolder(context.Background(), "", "foo/bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(folder, gc.NotNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		{"CreateFolder", []interface{}{"foo"}},
		{"CreateFolder", []interface{}{"bar"}},
	})
}

func (s *clientSuite) TestMoveVMFolderInto(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.MoveVMFolderInto(context.Background(), "foo", "foo/bar")
	c.Assert(err, jc.ErrorIsNil)

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
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeControllerVmFolder"),
		retrievePropertiesStubCall("FakeHostFolder"),
		{"MoveIntoFolder_Task", nil},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
	})
}

func (s *clientSuite) TestMoveVMsInto(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.MoveVMsInto(
		context.Background(), "foo",
		types.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: "vm-0",
		},
		types.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: "vm-1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeHostFolder"),
		{"MoveIntoFolder_Task", nil},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
	})
}

func (s *clientSuite) TestRemoveVirtualMachines(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.RemoveVirtualMachines(context.Background(), "foo/bar/*")
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeControllerVmFolder"),
		retrievePropertiesStubCall("FakeModelVmFolder"),
		retrievePropertiesStubCall("FakeVmTemplate", "FakeVm0", "FakeVm1"),
		{"Destroy_Task", nil},
		{"Destroy_Task", nil},
		{"PowerOffVM_Task", nil},
		{"Destroy_Task", nil},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
	})
}

func (s *clientSuite) TestRemoveVirtualMachinesDestroyRace(c *gc.C) {
	s.roundTripper.taskError[destroyTask] = &types.LocalizedMethodFault{
		Fault: &types.ManagedObjectNotFound{},
	}
	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.RemoveVirtualMachines(context.Background(), "foo/bar/*")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestUpdateVirtualMachineExtraConfig(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	var vm mo.VirtualMachine
	vm.Self = types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: "FakeVm0",
	}
	err := client.UpdateVirtualMachineExtraConfig(
		context.Background(), &vm, map[string]string{
			"k0": "v0",
			"k1": "",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCallNames(c,
		"ReconfigVM_Task",
		"CreatePropertyCollector",
		"CreateFilter",
		"WaitForUpdatesEx",
	)
}

func (s *clientSuite) TestVirtualMachines(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	result, err := client.VirtualMachines(context.Background(), "foo/bar/*")
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeControllerVmFolder"),
		retrievePropertiesStubCall("FakeModelVmFolder"),
		retrievePropertiesStubCall("FakeVmTemplate"),
		retrievePropertiesStubCall("FakeVm0"),
		retrievePropertiesStubCall("FakeVm1"),
	})

	c.Assert(result, gc.HasLen, 3)
	c.Assert(result[0].Name, gc.Equals, "juju-vm-template")
	c.Assert(result[1].Name, gc.Equals, "juju-vm-0")
	c.Assert(result[2].Name, gc.Equals, "juju-vm-1")
}

func (s *clientSuite) TestDatastores(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	result, err := client.Datastores(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		makeStubCall("FindByInventoryPath", "FakeSearchIndex", "/dc0/datastore"),
		retrievePropertiesStubCall("FakeDatastoreFolder"),
		retrievePropertiesStubCall("FakeDatastore1", "FakeDatastore2"),
	})

	c.Assert(result, gc.HasLen, 2)
	c.Assert(result[0].Name, gc.Equals, "datastore1")
	c.Assert(result[1].Name, gc.Equals, "datastore2")
}

func (s *clientSuite) TestDeleteDatastoreFile(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.DeleteDatastoreFile(context.Background(), "[datastore1] file/path")
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		{"DeleteDatastoreFile", []interface{}{"[datastore1] file/path"}},
		{"CreatePropertyCollector", nil},
		{"CreateFilter", nil},
		{"WaitForUpdatesEx", nil},
	})
}

func (s *clientSuite) TestDeleteDatastoreFileNotFound(c *gc.C) {
	s.roundTripper.taskError[deleteDatastoreFileTask] = &types.LocalizedMethodFault{
		Fault: &types.FileNotFound{},
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.DeleteDatastoreFile(context.Background(), "[datastore1] file/path")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestDeleteDatastoreError(c *gc.C) {
	s.roundTripper.taskError[deleteDatastoreFileTask] = &types.LocalizedMethodFault{
		Fault:            &types.NotAuthenticated{},
		LocalizedMessage: "nope",
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.DeleteDatastoreFile(context.Background(), "[datastore1] file/path")
	c.Assert(err, gc.ErrorMatches, "nope")
}

func (s *clientSuite) TestResourcePools(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	result, err := client.ResourcePools(context.Background(), "z0/...")

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeHostFolder"),
		makeStubCall("FindByInventoryPath", "FakeSearchIndex", "/z0"),
		retrievePropertiesStubCall("z0"),
		retrievePropertiesStubCall("FakeResourcePool1"),
		retrievePropertiesStubCall("FakeResourcePoolParent"),
		retrievePropertiesStubCall("FakeResourcePoolChild"),
		retrievePropertiesStubCall("FakeResourcePoolOther"),
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 4)
	c.Check(result[0].InventoryPath, gc.Equals, "/z0/Resources")
	c.Check(result[1].InventoryPath, gc.Equals, "/z0/Resources/parent")
	c.Check(result[2].InventoryPath, gc.Equals, "/z0/Resources/parent/child")
	c.Check(result[3].InventoryPath, gc.Equals, "/z0/Resources/other")
}

func (s *clientSuite) TestUserHasRootLevelPrivilege(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	result, err := client.UserHasRootLevelPrivilege(context.Background(), "Some.Privilege")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, true)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeSessionManager"),
		{"HasPrivilegeOnEntities", []interface{}{
			"FakeAuthorizationManager",
			[]types.ManagedObjectReference{s.serviceContent.RootFolder},
			"session-key",
			[]string{"Some.Privilege"},
		}},
	})

	s.roundTripper.SetErrors(nil, permissionError)
	result, err = client.UserHasRootLevelPrivilege(context.Background(), "System.Read")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, false)

	s.roundTripper.SetErrors(nil, permissionError)
	_, err = client.UserHasRootLevelPrivilege(context.Background(), "Other.Privilege")
	c.Assert(err, gc.ErrorMatches, `checking for "Other.Privilege" privilege: ServerFaultCode: Permission to perform this operation was denied.`)
}

var permissionError = soap.WrapSoapFault(&soap.Fault{
	XMLName: xml.Name{Space: "http://schemas.xmlsoap.org/soap/envelope/", Local: "Fault"},
	Code:    "ServerFaultCode",
	String:  "Permission to perform this operation was denied.",
	Detail: struct {
		Fault types.AnyType "xml:\",any,typeattr\""
	}{
		Fault: types.NoPermission{
			SecurityError: types.SecurityError{},
			Object:        types.ManagedObjectReference{Type: "Folder", Value: "group-d1"},
			PrivilegeId:   "System.Read",
		},
	},
})

func fakeAcquire(spec mutex.Spec) (func(), error) {
	return func() {}, nil
}
