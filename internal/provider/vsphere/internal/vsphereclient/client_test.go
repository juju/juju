// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/mutex/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"
	"golang.org/x/net/context"

	loggertesting "github.com/juju/juju/internal/logger/testing"
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

var _ = tc.Suite(&clientSuite{})

func (s *clientSuite) SetUpTest(c *tc.C) {
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
				{Name: "summary.config.template", Val: true},
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
				{Name: "config.version", Val: "vmx-10"},
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
				{Name: "config.version", Val: "vmx-10"},
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
				{Name: "config.version", Val: "vmx-10"},
				{Name: "summary.config.template", Val: true},
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
		logger.Infof(r.Context(), "HTTP %#v", r)
		var buf bytes.Buffer
		io.Copy(&buf, r.Body)
		rcopy := *r
		rcopy.Body = io.NopCloser(&buf)
		s.uploadRequests = append(s.uploadRequests, &rcopy)
		if s.onImageUpload != nil {
			s.onImageUpload(r)
		}
	}))
	s.AddCleanup(func(*tc.C) {
		s.server.Close()
	})
	s.roundTripper.serverURL = s.server.URL
	s.clock = testclock.NewClock(time.Now())
}

func (s *clientSuite) newFakeClient(c *tc.C, roundTripper soap.RoundTripper, dc string) *Client {
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
		logger:       loggertesting.WrapCheckLog(c),
		clock:        s.clock,
		acquireMutex: fakeAcquire,
	}
}

func (s *clientSuite) TestDial(c *tc.C) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		e := xml.NewEncoder(w)
		e.Encode(soap.Envelope{Body: methods.RetrieveServiceContentBody{
			Res: &types.RetrieveServiceContentResponse{
				Returnval: s.serviceContent,
			},
		}})
		e.Flush()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	url, err := url.Parse(server.URL)
	c.Assert(err, jc.ErrorIsNil)

	ctx := context.Background()
	client, err := Dial(ctx, url, "dc", loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(client, tc.NotNil)
}

func (s *clientSuite) TestClose(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	err := client.Close(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	s.roundTripper.CheckCallNames(c, "Logout")
}

func (s *clientSuite) TestComputeResources(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	result, err := client.ComputeResources(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeHostFolder"),
	})

	c.Assert(result, tc.HasLen, 2)
	c.Assert(result[0].Resource.Name, tc.Equals, "z0")
	c.Assert(result[0].Path, tc.Equals, "/dc0/host/z0")
	c.Assert(result[1].Resource.Name, tc.Equals, "z1")
	c.Assert(result[1].Path, tc.Equals, "/dc0/host/z1")
}

func (s *clientSuite) TestFolders(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	result, err := client.Folders(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
	})

	c.Assert(result.DatastoreFolder.InventoryPath, tc.Equals, "/dc0/datastore")
	c.Assert(result.HostFolder.InventoryPath, tc.Equals, "/dc0/host")
	c.Assert(result.NetworkFolder.InventoryPath, tc.Equals, "/dc0/network")
	c.Assert(result.VmFolder.InventoryPath, tc.Equals, "/dc0/vm")
}

func (s *clientSuite) TestDestroyVMFolder(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
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
		{FuncName: "Destroy_Task", Args: nil},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
	})
}

func (s *clientSuite) TestDestroyVMFolderRace(c *tc.C) {
	s.roundTripper.taskError[destroyTask] = &types.LocalizedMethodFault{
		Fault: &types.ManagedObjectNotFound{},
	}
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	err := client.DestroyVMFolder(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestEnsureVMFolder(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	folder, err := client.EnsureVMFolder(context.Background(), "", "foo/bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(folder, tc.NotNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		{FuncName: "CreateFolder", Args: []interface{}{"foo"}},
		{FuncName: "CreateFolder", Args: []interface{}{"bar"}},
	})
}

func (s *clientSuite) TestFindFolder(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	folder, err := client.FindFolder(context.Background(), "foo/bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(folder, tc.NotNil)
	c.Assert(folder.InventoryPath, tc.Equals, "/dc0/vm/foo/bar")
}

func (s *clientSuite) TestFindFolderRelativePath(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	folder, err := client.FindFolder(context.Background(), "./foo/bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(folder, tc.NotNil)
	c.Assert(folder.InventoryPath, tc.Equals, "/dc0/vm/foo/bar")
}

func (s *clientSuite) TestFindFolderAbsolutePath(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	_, err := client.FindFolder(context.Background(), "/foo/bar")
	// mock not set up to have a /foo/bar folder
	c.Assert(err, tc.ErrorMatches, `folder path "/foo/bar" not found`)
}

func (s *clientSuite) TestFindFolderSubPath(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	folder, err := client.FindFolder(context.Background(), "/dc0/vm/foo/bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(folder, tc.NotNil)
	c.Assert(folder.InventoryPath, tc.Equals, "/dc0/vm/foo/bar")
}

func (s *clientSuite) TestMoveVMFolderInto(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
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
		{FuncName: "MoveIntoFolder_Task", Args: nil},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
	})
}

func (s *clientSuite) TestMoveVMsInto(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
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
		{FuncName: "MoveIntoFolder_Task", Args: nil},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
	})
}

func (s *clientSuite) TestRemoveVirtualMachines(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
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
		{FuncName: "Destroy_Task", Args: nil},
		{FuncName: "Destroy_Task", Args: nil},
		{FuncName: "PowerOffVM_Task", Args: nil},
		{FuncName: "Destroy_Task", Args: nil},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
	})
}

func (s *clientSuite) TestRemoveVirtualMachinesDestroyRace(c *tc.C) {
	s.roundTripper.taskError[destroyTask] = &types.LocalizedMethodFault{
		Fault: &types.ManagedObjectNotFound{},
	}
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	err := client.RemoveVirtualMachines(context.Background(), "foo/bar/*")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestMaybeUpgradeVMVersionNotSet(c *tc.C) {
	args := CreateVirtualMachineParams{
		ForceVMHardwareVersion: 0,
	}
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	var vm mo.VirtualMachine
	vm.Self = types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: "FakeVm0",
	}

	vmObj := object.NewVirtualMachine(client.client.Client, vm.Reference())
	err := client.maybeUpgradeVMHardware(context.Background(), args, vmObj, &taskWaiter{})
	c.Assert(err, jc.ErrorIsNil)

	// No calls should be made. ForceVMHardwareVersion was not set.
	s.roundTripper.CheckCalls(c, []testing.StubCall{})
}

func (s *clientSuite) TestMaybeUpgradeVMVersionLowerThanSourceVM(c *tc.C) {
	args := CreateVirtualMachineParams{
		ForceVMHardwareVersion: 9,
	}
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	var vm mo.VirtualMachine
	vm.Self = types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: "FakeVm0",
	}

	vmObj := object.NewVirtualMachine(client.client.Client, vm.Reference())
	err := client.maybeUpgradeVMHardware(context.Background(), args, vmObj, &taskWaiter{})
	c.Assert(err, tc.ErrorMatches, `selected HW \(9\) version is lower than VM hardware`)

	// ForceVMHardwareVersion was set, but is lower than the VM version (vmx-10).
	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeVm0"),
	})
}

func (s *clientSuite) TestMaybeUpgradeVMVersionNotSupportedByEnv(c *tc.C) {
	// Version is mocked at vmx-13. Set a version larger than what env supports.
	args := CreateVirtualMachineParams{
		ForceVMHardwareVersion: 14,
		ComputeResource: &mo.ComputeResource{
			EnvironmentBrowser: &types.ManagedObjectReference{
				Type:  "EnvironmentBrowser",
				Value: "FakeEnvironmentBrowser",
			},
		},
	}
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	var vm mo.VirtualMachine
	vm.Self = types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: "FakeVm0",
	}

	vmObj := object.NewVirtualMachine(client.client.Client, vm.Reference())
	err := client.maybeUpgradeVMHardware(context.Background(), args, vmObj, &taskWaiter{})

	// We ignore the request and log the event.
	c.Assert(err, tc.ErrorMatches, `hardware version 14 not supported by target \(max version 13\)`)

	// No calls should be made. ForceVMHardwareVersion was not set.
	s.roundTripper.CheckCalls(c, []testing.StubCall{
		// Gets VM version
		retrievePropertiesStubCall("FakeVm0"),
		// Gets environment max version.
		{
			FuncName: "QueryConfigOption",
			Args: []interface{}{
				"FakeEnvironmentBrowser",
			},
		},
	})
}

func (s *clientSuite) TestMaybeUpgradeVMVersion(c *tc.C) {
	// Version is mocked at vmx-13. Set a version larger than what env supports.
	args := CreateVirtualMachineParams{
		// set the version to 11. This should prompt the upgrade.
		ForceVMHardwareVersion: 11,
		ComputeResource: &mo.ComputeResource{
			EnvironmentBrowser: &types.ManagedObjectReference{
				Type:  "EnvironmentBrowser",
				Value: "FakeEnvironmentBrowser",
			},
		},
		StatusUpdateParams: StatusUpdateParams{
			Clock:                  testclock.NewClock(time.Time{}),
			UpdateProgress:         func(status string) {},
			UpdateProgressInterval: time.Second,
		},
	}
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	var vm mo.VirtualMachine
	vm.Self = types.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: "FakeVm0",
	}

	vmObj := object.NewVirtualMachine(client.client.Client, vm.Reference())
	err := client.maybeUpgradeVMHardware(context.Background(), args, vmObj, &taskWaiter{
		clock:                  args.StatusUpdateParams.Clock,
		updateProgress:         args.StatusUpdateParams.UpdateProgress,
		updateProgressInterval: args.StatusUpdateParams.UpdateProgressInterval,
	})

	// We ignore the request and log the event.
	c.Assert(err, jc.ErrorIsNil)

	// No calls should be made. ForceVMHardwareVersion was not set.
	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeVm0"),
		{
			FuncName: "QueryConfigOption",
			Args: []interface{}{
				"FakeEnvironmentBrowser",
			},
		},
		{
			FuncName: "UpgradeVM_Task",
			Args: []interface{}{
				// must match the version we set in the model, if supported.
				"vmx-11",
			},
		},
		{
			FuncName: "CreatePropertyCollector",
			Args:     nil,
		},
		{
			FuncName: "CreateFilter",
			Args:     nil,
		},
		{
			FuncName: "WaitForUpdatesEx",
			Args:     nil,
		},
	})
}

func (s *clientSuite) TestUpdateVirtualMachineExtraConfig(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
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

func (s *clientSuite) TestVirtualMachines(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
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

	c.Assert(result, tc.HasLen, 3)
	c.Assert(result[0].Name, tc.Equals, "juju-vm-template")
	c.Assert(result[1].Name, tc.Equals, "juju-vm-0")
	c.Assert(result[2].Name, tc.Equals, "juju-vm-1")
}

func (s *clientSuite) TestListVMTemplates(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	result, err := client.ListVMTemplates(context.Background(), "foo/bar/*")
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

	c.Assert(result, tc.HasLen, 1)
	c.Assert(result[0].Name(), tc.Equals, "juju-vm-template")
}

func (s *clientSuite) TestDatastores(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
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

	c.Assert(result, tc.HasLen, 2)
	c.Assert(result[0].Name, tc.Equals, "datastore1")
	c.Assert(result[1].Name, tc.Equals, "datastore2")
}

func (s *clientSuite) TestDeleteDatastoreFile(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	err := client.DeleteDatastoreFile(context.Background(), "[datastore1] file/path")
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		{FuncName: "DeleteDatastoreFile", Args: []interface{}{"[datastore1] file/path"}},
		{FuncName: "CreatePropertyCollector", Args: nil},
		{FuncName: "CreateFilter", Args: nil},
		{FuncName: "WaitForUpdatesEx", Args: nil},
	})
}

func (s *clientSuite) TestDeleteDatastoreFileNotFound(c *tc.C) {
	s.roundTripper.taskError[deleteDatastoreFileTask] = &types.LocalizedMethodFault{
		Fault: &types.FileNotFound{},
	}

	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	err := client.DeleteDatastoreFile(context.Background(), "[datastore1] file/path")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestDeleteDatastoreError(c *tc.C) {
	s.roundTripper.taskError[deleteDatastoreFileTask] = &types.LocalizedMethodFault{
		Fault:            &types.NotAuthenticated{},
		LocalizedMessage: "nope",
	}

	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	err := client.DeleteDatastoreFile(context.Background(), "[datastore1] file/path")
	c.Assert(err, tc.ErrorMatches, "nope")
}

func (s *clientSuite) TestResourcePools(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
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
	c.Assert(result, tc.HasLen, 4)
	c.Check(result[0].InventoryPath, tc.Equals, "/z0/Resources")
	c.Check(result[1].InventoryPath, tc.Equals, "/z0/Resources/parent")
	c.Check(result[2].InventoryPath, tc.Equals, "/z0/Resources/parent/child")
	c.Check(result[3].InventoryPath, tc.Equals, "/z0/Resources/other")
}

func (s *clientSuite) TestUserHasRootLevelPrivilege(c *tc.C) {
	client := s.newFakeClient(c, &s.roundTripper, "dc0")
	result, err := client.UserHasRootLevelPrivilege(context.Background(), "Some.Privilege")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.Equals, true)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeSessionManager"),
		{FuncName: "HasPrivilegeOnEntities", Args: []interface{}{
			"FakeAuthorizationManager",
			[]types.ManagedObjectReference{s.serviceContent.RootFolder},
			"session-key",
			[]string{"Some.Privilege"},
		}},
	})

	s.roundTripper.SetErrors(nil, permissionError)
	result, err = client.UserHasRootLevelPrivilege(context.Background(), "System.Read")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.Equals, false)

	s.roundTripper.SetErrors(nil, permissionError)
	_, err = client.UserHasRootLevelPrivilege(context.Background(), "Other.Privilege")
	c.Assert(err, tc.ErrorMatches, `checking for "Other.Privilege" privilege: ServerFaultCode: Permission to perform this operation was denied.`)
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
			Object:        &types.ManagedObjectReference{Type: "Folder", Value: "group-d1"},
			PrivilegeId:   "System.Read",
		},
	},
})

func fakeAcquire(spec mutex.Spec) (func(), error) {
	return func() {}, nil
}
