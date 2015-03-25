// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vmware_test

import (
	"io/ioutil"
	"net/http"

	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/vmware"
)

type environBrokerSuite struct {
	vmware.BaseSuite

	hardware      *instance.HardwareCharacteristics
	spec          *instances.InstanceSpec
	ic            *instances.InstanceConstraint
	imageMetadata []*imagemetadata.ImageMetadata
	resolveInfo   *simplestreams.ResolveInfo
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *environBrokerSuite) TestStartInstance(c *gc.C) {
	client := vmware.ExposeEnvFakeClient(s.Env)
	client.SetPropertyProxyHandler("FakeDatacenter", vmware.RetrieveDatacenterProperties)
	client.SetProxyHandler("CreateImportSpec", func(req, res soap.HasFault) {
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
	client.SetProxyHandler("ImportVApp", func(req, res soap.HasFault) {
		resBody := res.(*methods.ImportVAppBody)
		resBody.Res = &types.ImportVAppResponse{
			Returnval: lease,
		}
	})
	client.SetProxyHandler("CreatePropertyCollector", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreatePropertyCollectorBody)
		resBody.Res = &types.CreatePropertyCollectorResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "",
				Value: "",
			},
		}
	})
	client.SetProxyHandler("CreateFilter", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreateFilterBody)
		resBody.Res = &types.CreateFilterResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "",
				Value: "",
			},
		}
	})
	client.SetProxyHandler("WaitForUpdatesEx", func(req, res soap.HasFault) {
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
													Url:       s.ServerUrl + "/disk-device/",
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
	client.SetProxyHandler("DestroyPropertyCollector", func(req, res soap.HasFault) {
		resBody := res.(*methods.DestroyPropertyCollectorBody)
		resBody.Res = &types.DestroyPropertyCollectorResponse{}
	})
	client.SetProxyHandler("HttpNfcLeaseComplete", func(req, res soap.HasFault) {
		resBody := res.(*methods.HttpNfcLeaseCompleteBody)
		resBody.Res = &types.HttpNfcLeaseCompleteResponse{}
	})
	powerOnTask := types.ManagedObjectReference{}
	client.SetProxyHandler("PowerOnVM_Task", func(req, res soap.HasFault) {
		resBody := res.(*methods.PowerOnVM_TaskBody)
		resBody.Res = &types.PowerOnVM_TaskResponse{
			Returnval: powerOnTask,
		}
	})
	client.SetProxyHandler("CreatePropertyCollector", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreatePropertyCollectorBody)
		resBody.Res = &types.CreatePropertyCollectorResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "",
				Value: "",
			},
		}
	})
	client.SetProxyHandler("CreateFilter", func(req, res soap.HasFault) {
		resBody := res.(*methods.CreateFilterBody)
		resBody.Res = &types.CreateFilterResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "",
				Value: "",
			},
		}
	})
	client.SetProxyHandler("WaitForUpdatesEx", func(req, res soap.HasFault) {
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
	client.SetProxyHandler("DestroyPropertyCollector", func(req, res soap.HasFault) {
		resBody := res.(*methods.DestroyPropertyCollectorBody)
		resBody.Res = &types.DestroyPropertyCollectorResponse{}
	})
	client.SetPropertyProxyHandler("", func(reqBody, resBody *methods.RetrievePropertiesBody) {
		vmware.CommonRetrieveProperties(resBody, "VirtualMachine", "FakeWirtualMachine", "name", "vm1")
	})

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
	s.ServeMux.HandleFunc("/disk-device/", func(w http.ResponseWriter, req *http.Request) {
		r, err := ioutil.ReadAll(req.Body)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(r), gc.Equals, "FakeVmdkContent")
	})
	_, err := s.Env.StartInstance(s.StartInstArgs)

	c.Assert(err, jc.ErrorIsNil)
}
