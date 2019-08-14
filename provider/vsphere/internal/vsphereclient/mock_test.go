// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	"github.com/juju/utils"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

var logger = loggo.GetLogger("vsphereclient")

var (
	lease = types.ManagedObjectReference{
		Type:  "Lease",
		Value: "FakeLease",
	}
	reconfigVMTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "ReconfigVMTask",
	}
	destroyTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "DestroyTask",
	}
	moveIntoFolderTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "MoveIntoFolder",
	}
	powerOffVMTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "PowerOffVM",
	}
	powerOnVMTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "PowerOnVM",
	}
	cloneVMTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "CloneVM",
	}
	searchDatastoreTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "SearchDatastore",
	}
	deleteDatastoreFileTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "DeleteDatastoreFile",
	}
	moveDatastoreFileTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "MoveDatastoreFile",
	}
	extendVirtualDiskTask = types.ManagedObjectReference{
		Type:  "Task",
		Value: "ExtendVirtualDisk",
	}
)

type mockRoundTripper struct {
	testing.Stub

	serverURL string
	roundTrip func(ctx context.Context, req, res soap.HasFault) error

	// mu protects access to the contents so we can change it in a
	// test.
	mu       sync.Mutex
	contents map[string][]types.ObjectContent

	collectors       map[string]*collector
	importVAppResult types.ManagedObjectReference
	taskError        map[types.ManagedObjectReference]*types.LocalizedMethodFault
	taskResult       map[types.ManagedObjectReference]types.AnyType
}

func (r *mockRoundTripper) RoundTrip(ctx context.Context, req, res soap.HasFault) error {
	if err := r.NextErr(); err != nil {
		return err
	}

	if r.roundTrip != nil {
		return r.roundTrip(ctx, req, res)
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
	case *methods.CloneVM_TaskBody:
		req := req.(*methods.CloneVM_TaskBody).Req
		specConfig := req.Spec.Config
		var vAppConfig types.BaseVmConfigSpec
		if specConfig != nil {
			vAppConfig = specConfig.VAppConfig
		}
		r.MethodCall(r, "CloneVM_Task", vAppConfig)
		res.Res = &types.CloneVM_TaskResponse{cloneVMTask}
	case *methods.CreateFolderBody:
		req := req.(*methods.CreateFolderBody).Req
		logger.Debugf("CreateFolder: %q", req.Name)
		r.MethodCall(r, "CreateFolder", req.Name)
		res.Res = &types.CreateFolderResponse{}
	case *methods.CreateImportSpecBody:
		req := req.(*methods.CreateImportSpecBody).Req
		r.MethodCall(r, "CreateImportSpec", req.OvfDescriptor, req.Datastore, req.Cisp)
		res.Res = &types.CreateImportSpecResponse{
			types.OvfCreateImportSpecResult{
				FileItem: []types.OvfFileItem{
					{
						DeviceId: "key1",
						Path:     "ubuntu-14.04-server-cloudimg-amd64.vmdk",
						Size:     14,
					},
				},
				ImportSpec: &types.VirtualMachineImportSpec{
					ConfigSpec: types.VirtualMachineConfigSpec{
						Name: "vm-name",
					},
				},
			},
		}
	case *methods.ImportVAppBody:
		req := req.(*methods.ImportVAppBody).Req
		r.MethodCall(r, "ImportVApp", req.Spec)
		res.Res = &types.ImportVAppResponse{lease}
	case *methods.SearchDatastore_TaskBody:
		req := req.(*methods.SearchDatastore_TaskBody).Req
		r.MethodCall(r, "SearchDatastore", req.DatastorePath, req.SearchSpec)
		res.Res = &types.SearchDatastore_TaskResponse{searchDatastoreTask}
	case *methods.DeleteDatastoreFile_TaskBody:
		req := req.(*methods.DeleteDatastoreFile_TaskBody).Req
		r.MethodCall(r, "DeleteDatastoreFile", req.Name)
		res.Res = &types.DeleteDatastoreFile_TaskResponse{deleteDatastoreFileTask}
	case *methods.MoveDatastoreFile_TaskBody:
		req := req.(*methods.MoveDatastoreFile_TaskBody).Req
		r.MethodCall(r, "MoveDatastoreFile", req.SourceName, req.DestinationName, req.Force)
		res.Res = &types.MoveDatastoreFile_TaskResponse{moveDatastoreFileTask}
	case *methods.MakeDirectoryBody:
		req := req.(*methods.MakeDirectoryBody).Req
		r.MethodCall(r, "MakeDirectory", req.Name)
		res.Res = &types.MakeDirectoryResponse{}
	case *methods.ExtendVirtualDisk_TaskBody:
		req := req.(*methods.ExtendVirtualDisk_TaskBody).Req
		r.MethodCall(r, "ExtendVirtualDisk", req.Name, req.NewCapacityKb)
		res.Res = &types.ExtendVirtualDisk_TaskResponse{extendVirtualDiskTask}
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
	case *methods.DestroyPropertyCollectorBody:
		req := req.(*methods.DestroyPropertyCollectorBody).Req
		delete(r.collectors, req.This.Value)
	case *methods.CreateFilterBody:
		r.MethodCall(r, "CreateFilter")
		req := req.(*methods.CreateFilterBody).Req
		r.collectors[req.This.Value].filter = req.Spec
		res.Res = &types.CreateFilterResponse{
			Returnval: req.Spec.ObjectSet[0].Obj,
		}
	case *methods.HttpNfcLeaseProgressBody:
		req := req.(*methods.HttpNfcLeaseProgressBody).Req
		r.MethodCall(r, "HttpNfcLeaseProgressBody", req.This.Value)
		logger.Infof("%s", req.This.Value)
		//delete(r.collectors, req.This.Value)
		res.Res = &types.HttpNfcLeaseProgressResponse{}
	case *methods.HttpNfcLeaseCompleteBody:
		req := req.(*methods.HttpNfcLeaseCompleteBody).Req
		r.MethodCall(r, "HttpNfcLeaseComplete", req.This.Value)
		delete(r.collectors, req.This.Value)
		res.Res = &types.HttpNfcLeaseCompleteResponse{}
	case *methods.WaitForUpdatesExBody:
		r.MethodCall(r, "WaitForUpdatesEx")
		req := req.(*methods.WaitForUpdatesExBody).Req
		collector := r.collectors[req.This.Value]

		var changes []types.PropertyChange
		if collector.filter.ObjectSet[0].Obj == lease {
			changes = []types.PropertyChange{{
				Name: "info",
				Val: types.HttpNfcLeaseInfo{
					Entity: r.importVAppResult,
					DeviceUrl: []types.HttpNfcLeaseDeviceUrl{
						{
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
			task := collector.filter.ObjectSet[0].Obj
			taskState := types.TaskInfoStateSuccess
			taskResult := r.taskResult[task]
			taskError := r.taskError[task]
			if taskError != nil {
				taskState = types.TaskInfoStateError
			}
			changes = []types.PropertyChange{{
				Name: "info",
				Op:   types.PropertyChangeOpAssign,
				Val: types.TaskInfo{
					Entity: &types.ManagedObjectReference{},
					State:  taskState,
					Result: taskResult,
					Error:  taskError,
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

	case *methods.FindByInventoryPathBody:
		req := req.(*methods.FindByInventoryPathBody).Req
		r.MethodCall(r, "FindByInventoryPath", req.This.Value, req.InventoryPath)
		logger.Debugf("FindByInventoryPath ref: %q, path: %q", req.This.Value, req.InventoryPath)
		res.Res = &types.FindByInventoryPathResponse{
			Returnval: &types.ManagedObjectReference{
				Type:  "ComputeResource",
				Value: "z0",
			},
		}
	case *methods.MarkAsTemplateBody:
		req := req.(*methods.MarkAsTemplateBody).Req
		r.MethodCall(r, "MarkAsTemplateBody", req.This.Value)
		res.Res = &types.MarkAsTemplateResponse{}

	default:
		logger.Debugf("mockRoundTripper: unknown res type %T", res)
		panic(fmt.Sprintf("unknown type %T", res))
		//		return errors.Errorf("unknown type %T", res)
	}
	return nil
}

func (r *mockRoundTripper) retrieveProperties(req *types.RetrieveProperties) *types.RetrievePropertiesResponse {
	spec := req.SpecSet[0]
	var args []interface{}
	for _, obj := range spec.ObjectSet {
		args = append(args, obj.Obj.Value)
	}
	var typeNames []string
	for _, prop := range spec.PropSet {
		typeNames = append(typeNames, prop.Type)
	}
	r.MethodCall(r, "RetrieveProperties", args...)
	logger.Debugf("RetrieveProperties for %s expecting %v", args, typeNames)
	r.mu.Lock()
	defer r.mu.Unlock()
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
	logger.Debugf("received %s", contents)
	return &types.RetrievePropertiesResponse{contents}
}

func (r *mockRoundTripper) setContents(contents map[string][]types.ObjectContent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contents = contents
}

func (r *mockRoundTripper) updateContents(key string, content []types.ObjectContent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contents[key] = content
}

func retrievePropertiesStubCall(vals ...string) testing.StubCall {
	return makeStubCall("RetrieveProperties", vals...)
}

func makeStubCall(method string, vals ...string) testing.StubCall {
	args := make([]interface{}, len(vals))
	for i, vals := range vals {
		args[i] = vals
	}
	return testing.StubCall{method, args}
}

type collector struct {
	filter types.PropertyFilterSpec
}
