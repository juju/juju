// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"github.com/juju/loggo"
	"github.com/juju/testing"
	"github.com/juju/utils"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

var logger = loggo.GetLogger("vsphereclient")

type mockRoundTripper struct {
	testing.Stub

	serverURL     string
	roundTrip     func(ctx context.Context, req, res soap.HasFault) error
	contents      map[string][]types.ObjectContent
	collectors    map[string]*collector
	leaseProgress chan int32
}

func (r *mockRoundTripper) RoundTrip(ctx context.Context, req, res soap.HasFault) error {
	if err := r.NextErr(); err != nil {
		return err
	}

	if r.roundTrip != nil {
		return r.roundTrip(ctx, req, res)
	}

	lease := types.ManagedObjectReference{
		Type:  "Lease",
		Value: "FakeLease",
	}
	reconfigVMTask := types.ManagedObjectReference{
		Type:  "Task",
		Value: "ReconfigVMTask",
	}
	destroyTask := types.ManagedObjectReference{
		Type:  "Task",
		Value: "DestroyTask",
	}
	moveIntoFolderTask := types.ManagedObjectReference{
		Type:  "Task",
		Value: "MoveIntoFolder",
	}
	powerOffVMTask := types.ManagedObjectReference{
		Type:  "Task",
		Value: "PowerOffVM",
	}
	powerOnVMTask := types.ManagedObjectReference{
		Type:  "Task",
		Value: "PowerOnVM",
	}

	switch res := res.(type) {
	case *methods.RetrievePropertiesBody:
		req := req.(*methods.RetrievePropertiesBody).Req
		res.Res = r.retrieveProperties(req)
	case *methods.LogoutBody:
		r.MethodCall(r, "Logout")
		res.Res = &types.LogoutResponse{}
	case *methods.ReconfigVM_TaskBody:
		r.MethodCall(r, "ReconfigVM_Task")
		res.Res = &types.ReconfigVM_TaskResponse{reconfigVMTask}
	case *methods.Destroy_TaskBody:
		r.MethodCall(r, "Destroy_Task")
		res.Res = &types.Destroy_TaskResponse{destroyTask}
	case *methods.MoveIntoFolder_TaskBody:
		r.MethodCall(r, "MoveIntoFolder_Task")
		res.Res = &types.MoveIntoFolder_TaskResponse{moveIntoFolderTask}
	case *methods.PowerOffVM_TaskBody:
		r.MethodCall(r, "PowerOffVM_Task")
		res.Res = &types.PowerOffVM_TaskResponse{powerOffVMTask}
	case *methods.PowerOnVM_TaskBody:
		r.MethodCall(r, "PowerOnVM_Task")
		res.Res = &types.PowerOnVM_TaskResponse{powerOnVMTask}
	case *methods.CreateFolderBody:
		r.MethodCall(r, "CreateFolder")
		res.Res = &types.CreateFolderResponse{}
	case *methods.CreateImportSpecBody:
		req := req.(*methods.CreateImportSpecBody).Req
		r.MethodCall(r, "CreateImportSpec", req.OvfDescriptor, req.Datastore, req.Cisp)
		res.Res = &types.CreateImportSpecResponse{
			types.OvfCreateImportSpecResult{
				FileItem: []types.OvfFileItem{
					types.OvfFileItem{
						DeviceId: "key1",
						Path:     "ubuntu-14.04-server-cloudimg-amd64.vmdk",
						Size:     14,
					},
				},
				ImportSpec: &types.VirtualMachineImportSpec{},
			},
		}
	case *methods.ImportVAppBody:
		req := req.(*methods.ImportVAppBody).Req
		r.MethodCall(r, "ImportVApp", req.Spec)
		res.Res = &types.ImportVAppResponse{lease}
	case *methods.CreatePropertyCollectorBody:
		r.MethodCall(r, "CreatePropertyCollector")
		uuid := utils.MustNewUUID().String()
		r.collectors[uuid] = &collector{}
		res.Res = &types.CreatePropertyCollectorResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "PropertyCollector",
				Value: uuid,
			},
		}
	case *methods.CreateFilterBody:
		r.MethodCall(r, "CreateFilter")
		req := req.(*methods.CreateFilterBody).Req
		r.collectors[req.This.Value].filter = req.Spec
		res.Res = &types.CreateFilterResponse{
			Returnval: req.Spec.ObjectSet[0].Obj,
		}
	case *methods.HttpNfcLeaseCompleteBody:
		req := req.(*methods.HttpNfcLeaseCompleteBody).Req
		r.MethodCall(r, "HttpNfcLeaseComplete", req.This.Value)
		delete(r.collectors, req.This.Value)
		res.Res = &types.HttpNfcLeaseCompleteResponse{}
	case *methods.HttpNfcLeaseProgressBody:
		req := req.(*methods.HttpNfcLeaseProgressBody).Req
		r.MethodCall(r, "HttpNfcLeaseProgress", req.This.Value, req.Percent)
		res.Res = &types.HttpNfcLeaseProgressResponse{}
		select {
		case r.leaseProgress <- req.Percent:
		default:
		}
	case *methods.WaitForUpdatesExBody:
		r.MethodCall(r, "WaitForUpdatesEx")
		req := req.(*methods.WaitForUpdatesExBody).Req
		collector := r.collectors[req.This.Value]

		var changes []types.PropertyChange
		if collector.filter.ObjectSet[0].Obj == lease {
			changes = []types.PropertyChange{{
				Name: "info",
				Val: types.HttpNfcLeaseInfo{
					DeviceUrl: []types.HttpNfcLeaseDeviceUrl{
						types.HttpNfcLeaseDeviceUrl{
							ImportKey: "key1",
							Url:       r.serverURL + "/disk-device/",
						},
					},
				},
			}, {
				Name: "state",
				Val:  types.HttpNfcLeaseStateReady,
			}}
		} else {
			changes = []types.PropertyChange{{
				Name: "info",
				Op:   types.PropertyChangeOpAssign,
				Val: types.TaskInfo{
					Entity: &types.ManagedObjectReference{},
					State:  types.TaskInfoStateSuccess,
				},
			}}
		}
		res.Res = &types.WaitForUpdatesExResponse{
			Returnval: &types.UpdateSet{
				FilterSet: []types.PropertyFilterUpdate{{
					ObjectSet: []types.ObjectUpdate{{
						Obj:       collector.filter.ObjectSet[0].Obj,
						ChangeSet: changes,
					}},
				}},
			},
		}

	default:
		return errors.Errorf("unknown type %T", res)
	}
	return nil
}

func (r *mockRoundTripper) retrieveProperties(req *types.RetrieveProperties) *types.RetrievePropertiesResponse {
	spec := req.SpecSet[0]
	var args []interface{}
	for _, obj := range spec.ObjectSet {
		args = append(args, obj.Obj.Value)
	}
	r.MethodCall(r, "RetrieveProperties", args...)
	logger.Debugf("RetrieveProperties for %s", args)
	var contents []types.ObjectContent
	for _, obj := range spec.ObjectSet {
		for _, content := range r.contents[obj.Obj.Value] {
			var match bool
			for _, prop := range spec.PropSet {
				if prop.Type == content.Obj.Type {
					match = true
					break
				}
			}
			if match {
				contents = append(contents, content)
			}
		}
	}
	return &types.RetrievePropertiesResponse{contents}
}

func retrievePropertiesStubCall(objs ...string) testing.StubCall {
	args := make([]interface{}, len(objs))
	for i, obj := range objs {
		args[i] = obj
	}
	return testing.StubCall{"RetrieveProperties", args}
}

type collector struct {
	filter types.PropertyFilterSpec
}
