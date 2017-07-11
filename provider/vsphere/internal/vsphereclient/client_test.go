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

	"github.com/juju/loggo"
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
		PropertyCollector: types.ManagedObjectReference{
			Type:  "PropertyCollector",
			Value: "FakePropertyCollector",
		},
	}
	s.roundTripper = mockRoundTripper{
		collectors:    make(map[string]*collector),
		leaseProgress: make(chan int32, 2),
	}
	s.roundTripper.contents = map[string][]types.ObjectContent{
		"FakeRootFolder": []types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "Datacenter",
				Value: "FakeDatacenter",
			},
			PropSet: []types.DynamicProperty{
				types.DynamicProperty{Name: "name", Val: "dc0"},
			},
		}},
		"FakeDatacenter": []types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "Datacenter",
				Value: "FakeDatacenter",
			},
			PropSet: []types.DynamicProperty{
				types.DynamicProperty{Name: "name", Val: "dc0"},
				types.DynamicProperty{Name: "hostFolder", Val: types.ManagedObjectReference{
					Type:  "Folder",
					Value: "FakeHostFolder",
				}},
				types.DynamicProperty{Name: "vmFolder", Val: types.ManagedObjectReference{
					Type:  "Folder",
					Value: "FakeVmFolder",
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
		"FakeHostFolder": []types.ObjectContent{{
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
		"FakeVmFolder": []types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "Folder",
				Value: "FakeControllerVmFolder",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "foo"},
			},
		}},
		"FakeControllerVmFolder": []types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "Folder",
				Value: "FakeModelVmFolder",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "bar"},
			},
		}},
		"FakeModelVmFolder": []types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: "FakeVm0",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "vm-0"},
			},
		}, {
			Obj: types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: "FakeVm1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "vm-1"},
			},
		}},
		"FakeVm0": []types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: "FakeVm0",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "vm-0"},
				{Name: "runtime.powerState", Val: "poweredOff"},
				{
					Name: "resourcePool",
					Val: types.ManagedObjectReference{
						Type:  "ResourcePool",
						Value: "FakeResourcePool0",
					},
				},
			},
		}},
		"FakeVm1": []types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: "FakeVm1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "vm-1"},
				{Name: "runtime.powerState", Val: "poweredOn"},
				{
					Name: "resourcePool",
					Val: types.ManagedObjectReference{
						Type:  "ResourcePool",
						Value: "FakeResourcePool1",
					},
				},
			},
		}},
		"FakeDatastore1": []types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "Datastore",
				Value: "FakeDatastore1",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "datastore1"},
				{Name: "summary.accessible", Val: false},
			},
		}},
		"FakeDatastore2": []types.ObjectContent{{
			Obj: types.ManagedObjectReference{
				Type:  "Datastore",
				Value: "FakeDatastore2",
			},
			PropSet: []types.DynamicProperty{
				{Name: "name", Val: "datastore2"},
				{Name: "summary.accessible", Val: true},
			},
		}},
	}

	// Create an HTTP server to receive image uploads.
	mux := http.NewServeMux()
	mux.HandleFunc("/disk-device/", func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		io.Copy(&buf, r.Body)
		rcopy := *r
		rcopy.Body = ioutil.NopCloser(&buf)
		s.uploadRequests = append(s.uploadRequests, &rcopy)
		if s.onImageUpload != nil {
			s.onImageUpload(r)
		}
	})
	s.uploadRequests = nil
	s.onImageUpload = nil
	s.server = httptest.NewServer(mux)
	s.AddCleanup(func(*gc.C) {
		s.server.Close()
	})
	s.roundTripper.serverURL = s.server.URL
}

func (s *clientSuite) newFakeClient(roundTripper soap.RoundTripper, dc string) *Client {
	vimClient := &vim25.Client{
		Client:         soap.NewClient(&url.URL{}, true),
		ServiceContent: s.serviceContent,
		RoundTripper:   roundTripper,
	}
	return &Client{
		client: &govmomi.Client{
			Client:         vimClient,
			SessionManager: session.NewManager(vimClient),
		},
		datacenter: dc,
		logger:     loggo.GetLogger("vsphereclient"),
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
		testing.StubCall{"Destroy_Task", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},
	})
}

func (s *clientSuite) TestEnsureVMFolder(c *gc.C) {
	client := s.newFakeClient(&s.roundTripper, "dc0")
	err := client.EnsureVMFolder(context.Background(), "foo/bar")
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		testing.StubCall{"CreateFolder", nil},
		testing.StubCall{"CreateFolder", nil},
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
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeControllerVmFolder"),
		retrievePropertiesStubCall("FakeHostFolder"),
		testing.StubCall{"MoveIntoFolder_Task", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},
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
		testing.StubCall{"MoveIntoFolder_Task", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},
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
		retrievePropertiesStubCall("FakeVm0", "FakeVm1"),
		testing.StubCall{"Destroy_Task", nil},
		testing.StubCall{"PowerOffVM_Task", nil},
		testing.StubCall{"Destroy_Task", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},
	})
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
		retrievePropertiesStubCall("FakeVm0"),
		retrievePropertiesStubCall("FakeVm1"),
	})

	c.Assert(result, gc.HasLen, 2)
	c.Assert(result[0].Name, gc.Equals, "vm-0")
	c.Assert(result[1].Name, gc.Equals, "vm-1")
}
