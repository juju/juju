// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/tc"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

var logger = internallogger.GetLogger("vsphereclient")

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
	testhelpers.Stub

	c *tc.C

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
		req := req.(*methods.ReconfigVM_TaskBody).Req
		r.MethodCall(r, "ReconfigVM_Task", req.Spec)
		res.Res = &types.ReconfigVM_TaskResponse{Returnval: reconfigVMTask}
	case *methods.Destroy_TaskBody:
		r.MethodCall(r, "Destroy_Task")
		res.Res = &types.Destroy_TaskResponse{Returnval: destroyTask}
	case *methods.MoveIntoFolder_TaskBody:
		r.MethodCall(r, "MoveIntoFolder_Task")
		res.Res = &types.MoveIntoFolder_TaskResponse{Returnval: moveIntoFolderTask}
	case *methods.PowerOffVM_TaskBody:
		r.MethodCall(r, "PowerOffVM_Task")
		res.Res = &types.PowerOffVM_TaskResponse{Returnval: powerOffVMTask}
	case *methods.PowerOnVM_TaskBody:
		r.MethodCall(r, "PowerOnVM_Task")
		res.Res = &types.PowerOnVM_TaskResponse{Returnval: powerOnVMTask}
	case *methods.CloneVM_TaskBody:
		req := req.(*methods.CloneVM_TaskBody).Req
		r.MethodCall(r, "CloneVM_Task", req.Folder, req.Name, req.Spec.Config, req.Spec.Location)
		res.Res = &types.CloneVM_TaskResponse{Returnval: cloneVMTask}
	case *methods.CreateFolderBody:
		req := req.(*methods.CreateFolderBody).Req
		logger.Debugf(ctx, "CreateFolder: %q", req.Name)
		r.MethodCall(r, "CreateFolder", req.Name)
		res.Res = &types.CreateFolderResponse{}
	case *methods.CreateImportSpecBody:
		req := req.(*methods.CreateImportSpecBody).Req
		r.MethodCall(r, "CreateImportSpec", req.OvfDescriptor, req.Datastore, req.Cisp)
		res.Res = &types.CreateImportSpecResponse{
			Returnval: types.OvfCreateImportSpecResult{
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
		res.Res = &types.ImportVAppResponse{Returnval: lease}
	case *methods.SearchDatastore_TaskBody:
		req := req.(*methods.SearchDatastore_TaskBody).Req
		r.MethodCall(r, "SearchDatastore", req.DatastorePath, req.SearchSpec)
		res.Res = &types.SearchDatastore_TaskResponse{Returnval: searchDatastoreTask}
	case *methods.DeleteDatastoreFile_TaskBody:
		req := req.(*methods.DeleteDatastoreFile_TaskBody).Req
		r.MethodCall(r, "DeleteDatastoreFile", req.Name)
		res.Res = &types.DeleteDatastoreFile_TaskResponse{Returnval: deleteDatastoreFileTask}
	case *methods.MoveDatastoreFile_TaskBody:
		req := req.(*methods.MoveDatastoreFile_TaskBody).Req
		r.MethodCall(r, "MoveDatastoreFile", req.SourceName, req.DestinationName, req.Force)
		res.Res = &types.MoveDatastoreFile_TaskResponse{Returnval: moveDatastoreFileTask}
	case *methods.MakeDirectoryBody:
		req := req.(*methods.MakeDirectoryBody).Req
		r.MethodCall(r, "MakeDirectory", req.Name)
		res.Res = &types.MakeDirectoryResponse{}
	case *methods.ExtendVirtualDisk_TaskBody:
		req := req.(*methods.ExtendVirtualDisk_TaskBody).Req
		r.MethodCall(r, "ExtendVirtualDisk", req.Name, req.NewCapacityKb)
		res.Res = &types.ExtendVirtualDisk_TaskResponse{Returnval: extendVirtualDiskTask}
	case *methods.CreatePropertyCollectorBody:
		r.MethodCall(r, "CreatePropertyCollector")
		uuid := uuid.MustNewUUID().String()
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
		logger.Infof(ctx, "%s", req.This.Value)
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
		logger.Debugf(ctx, "FindByInventoryPath ref: %q, path: %q", req.This.Value, req.InventoryPath)
		var findResponse *types.FindByInventoryPathResponse
		if req.InventoryPath == "/dc0/datastore" {
			findResponse = &types.FindByInventoryPathResponse{
				Returnval: &types.ManagedObjectReference{
					Type:  "Folder",
					Value: "FakeDatastoreFolder",
				},
			}

		} else {
			findResponse = &types.FindByInventoryPathResponse{
				Returnval: &types.ManagedObjectReference{
					Type:  "ComputeResource",
					Value: "z0",
				},
			}
		}
		res.Res = findResponse
	case *methods.MarkAsTemplateBody:
		req := req.(*methods.MarkAsTemplateBody).Req
		r.MethodCall(r, "MarkAsTemplate", req.This.Value)
		res.Res = &types.MarkAsTemplateResponse{}

	case *methods.HasPrivilegeOnEntitiesBody:
		req := req.(*methods.HasPrivilegeOnEntitiesBody).Req
		r.MethodCall(r, "HasPrivilegeOnEntities", req.This.Value, req.Entity, req.SessionId, req.PrivId)
		res.Res = &types.HasPrivilegeOnEntitiesResponse{
			Returnval: []types.EntityPrivilege{{
				Entity: req.Entity[0],
				PrivAvailability: []types.PrivilegeAvailability{{
					PrivId:    req.PrivId[0],
					IsGranted: true,
				}},
			}},
		}
	case *methods.QueryConfigOptionBody:
		req := req.(*methods.QueryConfigOptionBody).Req
		r.MethodCall(r, "QueryConfigOption", req.This.Value)
		res.Res = &types.QueryConfigOptionResponse{
			Returnval: &types.VirtualMachineConfigOption{
				Version: "vmx-13",
			},
		}
	case *methods.UpgradeVM_TaskBody:
		req := req.(*methods.UpgradeVM_TaskBody).Req
		r.MethodCall(r, "UpgradeVM_Task", req.Version)
		res.Res = &types.UpgradeVM_TaskResponse{
			Returnval: types.ManagedObjectReference{
				Type:  "Task",
				Value: "UpgradeVM_Task",
			},
		}
	default:
		logger.Debugf(ctx, "mockRoundTripper: unknown res type %T", res)
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
	logger.Debugf(r.c.Context(), "RetrieveProperties for %s expecting %v", args, typeNames)
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
	logger.Debugf(r.c.Context(), "received %s", contents)
	return &types.RetrievePropertiesResponse{Returnval: contents}
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

func retrievePropertiesStubCall(vals ...string) testhelpers.StubCall {
	return makeStubCall("RetrieveProperties", vals...)
}

func makeStubCall(method string, vals ...string) testhelpers.StubCall {
	args := make([]interface{}, len(vals))
	for i, vals := range vals {
		args[i] = vals
	}
	return testhelpers.StubCall{FuncName: method, Args: args}
}

type collector struct {
	filter types.PropertyFilterSpec
}
