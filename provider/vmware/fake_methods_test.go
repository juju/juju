// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vmware

import (
	"io/ioutil"
	"net/http"

	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

func (s *BaseSuite) FakeMetadataServer() {
	s.ServeMux.HandleFunc("/streams/v1/index.json", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{
 "index": {
  "com.ubuntu.cloud:released:download": {
   "datatype": "image-downloads", 
   "path": "streams/v1/com.ubuntu.cloud:released:download.json", 
   "updated": "Tue, 24 Feb 2015 10:16:54 +0000", 
   "products": [
    "com.ubuntu.cloud:server:14.04:amd64" 
   ], 
   "format": "products:1.0"
  }
 }, 
 "updated": "Tue, 24 Feb 2015 14:14:24 +0000", 
 "format": "index:1.0"
}`))
	})
	s.ServeMux.HandleFunc("/streams/v1/com.ubuntu.cloud:released:download.json", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{
 "updated": "Thu, 05 Mar 2015 12:14:40 +0000", 
 "license": "http://www.canonical.com/intellectual-property-policy", 
 "format": "products:1.0", 
 "datatype": "image-downloads", 
 "products": {
    "com.ubuntu.cloud:server:14.04:amd64": {
      "release": "trusty", 
      "version": "14.04", 
      "arch": "amd64", 
      "versions": {
        "20150305": {
          "items": {
            "ovf": {
              "size": 7196, 
              "path": "server/releases/trusty/release-20150305/ubuntu-14.04-server-cloudimg-amd64.ovf", 
              "ftype": "ovf", 
              "sha256": "d6cade98b50e2e27f4508b01fea99d5b26a2f2a184c83e5fb597ca7b142ec01c", 
              "md5": "00662c59ca52558e7a3bb9a67d194730"
            }
          }      
        }
      }
    }
  }
}`))
	})
	s.ServeMux.HandleFunc("/server/releases/trusty/release-20150305/ubuntu-14.04-server-cloudimg-amd64.ovf", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("FakeOvfContent"))
	})
	s.ServeMux.HandleFunc("/server/releases/trusty/release-20150305/ubuntu-14.04-server-cloudimg-amd64.vmdk", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("FakeVmdkContent"))
	})
}

func (s *BaseSuite) FakeInstances(c *fakeClient) {
	c.SetPropertyProxyHandler("FakeVmFolder", func(reqBody, resBody *methods.RetrievePropertiesBody) {
		resBody.Res = &types.RetrievePropertiesResponse{
			Returnval: []types.ObjectContent{},
		}
	})
}

func (s *BaseSuite) FakeAvailabilityZones(c *fakeClient, zoneName ...string) {
	c.SetPropertyProxyHandler("FakeDatacenter", RetrieveDatacenterProperties)
	retVal := []types.ObjectContent{}
	for _, zone := range zoneName {
		retVal = append(retVal, types.ObjectContent{
			Obj: types.ManagedObjectReference{
				Type:  "ComputeResource",
				Value: zone,
			},
			PropSet: []types.DynamicProperty{
				{Name: "resourcePool", Val: types.ManagedObjectReference{
					Type:  "ResourcePool",
					Value: "FakeResourcePool",
				}},
				{Name: "name", Val: zone},
			},
		})
	}

	c.SetPropertyProxyHandler("FakeHostFolder", func(reqBody, resBody *methods.RetrievePropertiesBody) {
		resBody.Res = &types.RetrievePropertiesResponse{
			Returnval: retVal,
		}
	})
}

func (s *BaseSuite) FakeCreateInstance(c *fakeClient, serverUrl string) {
	s.FakeImportOvf(c, serverUrl)
	powerOnTask := types.ManagedObjectReference{}
	c.SetProxyHandler("PowerOnVM_Task", func(req, res soap.HasFault) {
		resBody := res.(*methods.PowerOnVM_TaskBody)
		resBody.Res = &types.PowerOnVM_TaskResponse{
			Returnval: powerOnTask,
		}
	})
	c.SetProxyHandler("CreatePropertyCollector", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreatePropertyCollectorBody)
		resBody.Res = &types.CreatePropertyCollectorResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "",
				Value: "",
			},
		}
	})
	c.SetProxyHandler("CreateFilter", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreateFilterBody)
		resBody.Res = &types.CreateFilterResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "",
				Value: "",
			},
		}
	})
	c.SetProxyHandler("WaitForUpdatesEx", func(req, res soap.HasFault) {
		resBody := res.(*methods.WaitForUpdatesExBody)
		resBody.Res = &types.WaitForUpdatesExResponse{
			Returnval: &types.UpdateSet{
				FilterSet: []types.PropertyFilterUpdate{
					types.PropertyFilterUpdate{
						ObjectSet: []types.ObjectUpdate{
							types.ObjectUpdate{
								Obj: powerOnTask,
								ChangeSet: []types.PropertyChange{
									types.PropertyChange{
										Name: "info",
										Op:   types.PropertyChangeOpAssign,
										Val: types.TaskInfo{
											Entity: &types.ManagedObjectReference{},
											State:  types.TaskInfoStateSuccess,
										},
									},
								},
							},
						},
					},
				},
			},
		}
	})
	c.SetProxyHandler("DestroyPropertyCollector", func(req, res soap.HasFault) {
		resBody := res.(*methods.DestroyPropertyCollectorBody)
		resBody.Res = &types.DestroyPropertyCollectorResponse{}
	})
	c.SetPropertyProxyHandler("", func(reqBody, resBody *methods.RetrievePropertiesBody) {
		CommonRetrieveProperties(resBody, "VirtualMachine", "FakeWirtualMachine", "name", "vm1")
	})
}

func (s *BaseSuite) FakeImportOvf(c *fakeClient, serverUrl string) {
	c.SetPropertyProxyHandler("FakeDatacenter", RetrieveDatacenterProperties)
	c.SetProxyHandler("CreateImportSpec", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreateImportSpecBody)
		resBody.Res = &types.CreateImportSpecResponse{
			Returnval: types.OvfCreateImportSpecResult{
				FileItem: []types.OvfFileItem{
					types.OvfFileItem{
						DeviceId: "key1",
						Path:     "ubuntu-14.04-server-cloudimg-amd64.vmdk",
					},
				},
				ImportSpec: &types.VirtualMachineImportSpec{},
			},
		}
	})
	lease := types.ManagedObjectReference{
		Type:  "Lease",
		Value: "FakeLease",
	}
	c.SetProxyHandler("ImportVApp", func(req, res soap.HasFault) {
		resBody := res.(*methods.ImportVAppBody)
		resBody.Res = &types.ImportVAppResponse{
			Returnval: lease,
		}
	})
	c.SetProxyHandler("CreatePropertyCollector", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreatePropertyCollectorBody)
		resBody.Res = &types.CreatePropertyCollectorResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "",
				Value: "",
			},
		}
	})
	c.SetProxyHandler("CreateFilter", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreateFilterBody)
		resBody.Res = &types.CreateFilterResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "",
				Value: "",
			},
		}
	})
	c.SetProxyHandler("WaitForUpdatesEx", func(req, res soap.HasFault) {
		resBody := res.(*methods.WaitForUpdatesExBody)
		resBody.Res = &types.WaitForUpdatesExResponse{
			Returnval: &types.UpdateSet{
				FilterSet: []types.PropertyFilterUpdate{
					types.PropertyFilterUpdate{
						ObjectSet: []types.ObjectUpdate{
							types.ObjectUpdate{
								Obj: lease,
								ChangeSet: []types.PropertyChange{
									types.PropertyChange{
										Name: "info",
										Val: types.HttpNfcLeaseInfo{
											DeviceUrl: []types.HttpNfcLeaseDeviceUrl{
												types.HttpNfcLeaseDeviceUrl{
													ImportKey: "key1",
													Url:       serverUrl + "/disk-device/",
												},
											},
										},
									},
									types.PropertyChange{
										Name: "state",
										Val:  types.HttpNfcLeaseStateReady,
									},
								},
							},
						},
					},
				},
			},
		}
	})
	s.ServeMux.HandleFunc("/disk-device/", func(w http.ResponseWriter, req *http.Request) {
		ioutil.ReadAll(req.Body)
		//r, err := ioutil.ReadAll(req.Body)
		//c.Assert(err, jc.ErrorIsNil)
		//c.Assert(string(r), gc.Equals, "FakeVmdkContent")
	})
	c.SetProxyHandler("DestroyPropertyCollector", func(req, res soap.HasFault) {
		resBody := res.(*methods.DestroyPropertyCollectorBody)
		resBody.Res = &types.DestroyPropertyCollectorResponse{}
	})
	c.SetProxyHandler("HttpNfcLeaseComplete", func(req, res soap.HasFault) {
		resBody := res.(*methods.HttpNfcLeaseCompleteBody)
		resBody.Res = &types.HttpNfcLeaseCompleteResponse{}
	})
}
