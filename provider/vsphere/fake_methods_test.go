// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"archive/tar"
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/juju/govmomi/vim25/methods"
	"github.com/juju/govmomi/vim25/soap"
	"github.com/juju/govmomi/vim25/types"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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
            "ova": {
              "size": 7196, 
              "path": "server/releases/trusty/release-20150305/ubuntu-14.04-server-cloudimg-amd64.ova", 
              "ftype": "ova", 
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
	s.ServeMux.HandleFunc("/server/releases/trusty/release-20150305/ubuntu-14.04-server-cloudimg-amd64.ova", func(w http.ResponseWriter, req *http.Request) {
		w.Write(s.createFakeOva())
	})
}

func (s *BaseSuite) createFakeOva() []byte {
	buf := new(bytes.Buffer)

	tw := tar.NewWriter(buf)

	var files = []struct {
		Name, Body string
	}{
		{"ubuntu-14.04-server-cloudimg-amd64.ovf", "FakeOvfContent"},
		{"ubuntu-14.04-server-cloudimg-amd64.vmdk", "FakeVmdkContent"},
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name: file.Name,
			Size: int64(len(file.Body)),
		}
		tw.WriteHeader(hdr)
		tw.Write([]byte(file.Body))
	}
	tw.Close()
	return buf.Bytes()

}

func (s *BaseSuite) FakeInstances(c *fakeClient, instName ...string) {
	c.SetPropertyProxyHandler("FakeVmFolder", func(reqBody, resBody *methods.RetrievePropertiesBody) {
		resBody.Res = &types.RetrievePropertiesResponse{
			Returnval: []types.ObjectContent{},
		}
	})
	c.SetPropertyProxyHandler("FakeVmFolder", func(reqBody, resBody *methods.RetrievePropertiesBody) {
		resBody.Res = &types.RetrievePropertiesResponse{
			Returnval: []types.ObjectContent{},
		}
	})
}

type InstRp struct {
	Inst, Rp string
}

func (s *BaseSuite) FakeInstancesWithResourcePool(c *fakeClient, instances ...InstRp) {
	retVal := []types.ObjectContent{}
	for _, vm := range instances {
		retVal = append(retVal, types.ObjectContent{
			Obj: types.ManagedObjectReference{
				Type:  "VirtualMachine",
				Value: vm.Inst,
			},
			PropSet: []types.DynamicProperty{
				{Name: "resourcePool", Val: types.ManagedObjectReference{
					Type:  "ResourcePool",
					Value: vm.Rp,
				}},
				{Name: "name", Val: vm.Inst},
			},
		})
	}
	c.SetPropertyProxyHandler("FakeVmFolder", func(reqBody, resBody *methods.RetrievePropertiesBody) {
		resBody.Res = &types.RetrievePropertiesResponse{
			Returnval: retVal,
		}
	})
	c.SetPropertyProxyHandler("FakeVmFolder", func(reqBody, resBody *methods.RetrievePropertiesBody) {
		resBody.Res = &types.RetrievePropertiesResponse{
			Returnval: retVal,
		}
	})
	results := []*types.RetrievePropertiesResponse{}
	for _, vm := range instances {
		results = append(results, &types.RetrievePropertiesResponse{
			Returnval: []types.ObjectContent{{
				Obj: types.ManagedObjectReference{
					Type:  "VirtualMachine",
					Value: vm.Inst,
				},
				PropSet: []types.DynamicProperty{
					{Name: "resourcePool", Val: types.ManagedObjectReference{
						Type:  "ResourcePool",
						Value: vm.Rp,
					}},
					{Name: "name", Val: vm.Inst},
				},
			}},
		})
		c.SetPropertyProxyHandler(vm.Inst, func(reqBody, resBody *methods.RetrievePropertiesBody) {
			for i, vm := range instances {
				if vm.Inst == reqBody.Req.SpecSet[0].ObjectSet[0].Obj.Value {
					resBody.Res = results[i]
					return
				}
			}
			panic("Match not found")
		})
	}
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
				{Name: "datastore", Val: []types.ManagedObjectReference{{
					Type:  "Datastore",
					Value: "FakeDatastore",
				}}},
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

type ZoneRp struct {
	Zone, Rp string
}

func (s *BaseSuite) FakeAvailabilityZonesWithResourcePool(c *fakeClient, zones ...ZoneRp) {
	c.SetPropertyProxyHandler("FakeDatacenter", RetrieveDatacenterProperties)
	retVal := []types.ObjectContent{}
	for _, zone := range zones {
		retVal = append(retVal, types.ObjectContent{
			Obj: types.ManagedObjectReference{
				Type:  "ComputeResource",
				Value: zone.Zone,
			},
			PropSet: []types.DynamicProperty{
				{Name: "resourcePool", Val: types.ManagedObjectReference{
					Type:  "ResourcePool",
					Value: zone.Rp,
				}},
				{Name: "datastore", Val: []types.ManagedObjectReference{{
					Type:  "Datastore",
					Value: "FakeDatastore",
				}}},
				{Name: "name", Val: zone.Zone},
			},
		})
	}

	c.SetPropertyProxyHandler("FakeHostFolder", func(reqBody, resBody *methods.RetrievePropertiesBody) {
		resBody.Res = &types.RetrievePropertiesResponse{
			Returnval: retVal,
		}
	})
}

func (s *BaseSuite) FakeCreateInstance(c *fakeClient, serverUrl string, checker *gc.C) {
	s.FakeImportOvf(c, serverUrl, checker)
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

func (s *BaseSuite) FakeImportOvf(c *fakeClient, serverUrl string, checker *gc.C) {
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
		r, err := ioutil.ReadAll(req.Body)
		checker.Assert(err, jc.ErrorIsNil)
		checker.Assert(string(r), gc.Equals, "FakeVmdkContent")
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
