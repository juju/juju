// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/storage"
)

// ContextInfo holds the values for the hook context.
type ContextInfo struct {
	*Unit
	*Status
	*Instance
	*NetworkInterface
	*Leadership
	*Storage
	*Relations
	*RelationHook
	*Action
}

// NewContextInfo returns a new ContextInfo.
func NewContextInfo() *ContextInfo {
	return &ContextInfo{
		Unit:             &Unit{},
		Status:           &Status{},
		Instance:         &Instance{},
		NetworkInterface: &NetworkInterface{},
		Leadership:       &Leadership{},
		Storage:          &Storage{},
		Relations:        &Relations{},
		RelationHook:     &RelationHook{},
		Action:           &Action{},
	}
}

// Update calls each of the provided update functions, which update
// the ContextInfo.
func (ci *ContextInfo) Update(updateFuncs ...func(*ContextInfo) error) error {
	for _, update := range updateFuncs {
		if err := update(ci); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Context is a test double for jujuc.Context.
type Context struct {
	*ContextUnit
	*ContextStatus
	*ContextInstance
	*ContextNetworking
	*ContextLeader
	*ContextMetrics
	*ContextStorage
	*ContextRelations
	*ContextAction

	Stub *testing.Stub
	Info *ContextInfo
}

// NewContext returns a new Context.
func NewContext(stub *testing.Stub, info *ContextInfo) *Context {
	ctx := &Context{
		Stub: stub,
		Info: info,
	}
	ctx.init()
	return ctx
}

func (ctx *Context) init() {
	if ctx.ContextUnit == nil {
		ctx.ContextUnit = &ContextUnit{}
	}
	if ctx.ContextStatus == nil {
		ctx.ContextStatus = &ContextStatus{}
	}
	if ctx.ContextInstance == nil {
		ctx.ContextInstance = &ContextInstance{}
	}
	if ctx.ContextNetworking == nil {
		ctx.ContextNetworking = &ContextNetworking{}
	}
	if ctx.ContextLeader == nil {
		ctx.ContextLeader = &ContextLeader{}
	}
	if ctx.ContextMetrics == nil {
		ctx.ContextMetrics = &ContextMetrics{}
	}
	if ctx.ContextStorage == nil {
		ctx.ContextStorage = &ContextStorage{}
	}
	if ctx.ContextRelations == nil {
		ctx.ContextRelations = &ContextRelations{}
	}
	if ctx.ContextAction == nil {
		ctx.ContextAction = &ContextAction{}
	}

	ctx.ensureStub()
	ctx.syncInfo()
}

func (ctx *Context) ensureStub() {
	if ctx.Stub == nil {
		ctx.Stub = &testing.Stub{}
	}
	stub := ctx.Stub

	if ctx.ContextUnit.Stub == nil {
		ctx.ContextUnit.Stub = stub
	}
	if ctx.ContextStatus.Stub == nil {
		ctx.ContextStatus.Stub = stub
	}
	if ctx.ContextInstance.Stub == nil {
		ctx.ContextInstance.Stub = stub
	}
	if ctx.ContextNetworking.Stub == nil {
		ctx.ContextNetworking.Stub = stub
	}
	if ctx.ContextLeader.Stub == nil {
		ctx.ContextLeader.Stub = stub
	}
	if ctx.ContextMetrics.Stub == nil {
		ctx.ContextMetrics.Stub = stub
	}
	if ctx.ContextStorage.Stub == nil {
		ctx.ContextStorage.Stub = stub
	}
	if ctx.ContextRelations.Stub == nil {
		ctx.ContextRelations.Stub = stub
	}
	if ctx.ContextAction.Stub == nil {
		ctx.ContextAction.Stub = stub
	}
}

func (ctx *Context) syncInfo() {
	if ctx.Info == nil {
		ctx.Info = &ContextInfo{}
	}
	info := ctx.Info

	if ctx.ContextUnit.Info == nil {
		ctx.ContextUnit.Info = info.Unit
	} else {
		info.Unit = ctx.ContextUnit.Info
	}

	if ctx.ContextStatus.Info == nil {
		ctx.ContextStatus.Info = info.Status
	} else {
		info.Status = ctx.ContextStatus.Info
	}

	if ctx.ContextInstance.Info == nil {
		ctx.ContextInstance.Info = info.Instance
	} else {
		info.Instance = ctx.ContextInstance.Info
	}

	if ctx.ContextNetworking.Info == nil {
		ctx.ContextNetworking.Info = info.NetworkInterface
	} else {
		info.NetworkInterface = ctx.ContextNetworking.Info
	}

	if ctx.ContextLeader.Info == nil {
		ctx.ContextLeader.Info = info.Leadership
	} else {
		info.Leadership = ctx.ContextLeader.Info
	}

	// There is no metrics info.

	if ctx.ContextStorage.Info == nil {
		ctx.ContextStorage.Info = info.Storage
	} else {
		info.Storage = ctx.ContextStorage.Info
	}

	if ctx.ContextRelations.Relations == nil {
		ctx.ContextRelations.Relations = info.Relations
	} else {
		info.Relations = ctx.ContextRelations.Relations
	}
	if ctx.ContextRelations.Hook == nil {
		ctx.ContextRelations.Hook = info.RelationHook
	} else {
		info.RelationHook = ctx.ContextRelations.Hook
	}

	if ctx.ContextAction.Info == nil {
		ctx.ContextAction.Info = info.Action
	} else {
		info.Action = ctx.ContextAction.Info
	}
}

func (ctx *Context) update(updateFuncs ...func(*Context) error) error {
	ctx.init()
	for _, update := range updateFuncs {
		if err := update(ctx); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (ctx *Context) updateInfo(updateFuncs ...func(*ContextInfo) error) error {
	ctx.init()
	return ctx.Info.Update(updateFuncs...)
}

func (ctx *Context) setRelation(id int, name, unit string, settings Settings) *Relation {
	ctx.init()
	relCtx := ctx.ContextRelations.setRelation(id, name)
	ctx.ContextRelations.setRelated(id, unit, settings)
	return relCtx.Info
}

func (ctx *Context) setStorage(name, location string, kind storage.StorageKind) {
	ctx.init()
	ctx.ContextStorage.setAttachment(name, location, kind)
}

// ContextWrapper provides helper functions around the embedded Context.
type ContextWrapper struct {
	*Context
}

// Update calls each of the provided update functions, which update
// the Context.
func (w *ContextWrapper) Update(updateFuncs ...func(*Context) error) error {
	return w.update(updateFuncs...)
}

// UpdateInfo calls each of the provided update functions, which update
// the ContextInfo.
func (w *ContextWrapper) UpdateInfo(updateFuncs ...func(*ContextInfo) error) error {
	return w.updateInfo(updateFuncs...)
}

// SetAsRelationHook updates the context to work as a relation hook context.
func (w *ContextWrapper) SetAsRelationHook(id int, remote string) {
	w.Info.HookRelation = id
	w.Info.RemoteUnitName = remote
}

// SetAsActionHook updates the context to work as an action hook context.
func (w *ContextWrapper) SetAsActionHook() {
	panic("not supported yet")
}

// SetRelation sets up the specified relation within the context.
func (w *ContextWrapper) SetRelation(id int, name, unit string, settings Settings) *Relation {
	return w.setRelation(id, name, unit, settings)
}

// SetRelated adds the provided unit information to the relation.
func (w *ContextWrapper) SetRelated(id int, unit string, settings Settings) {
	w.setRelated(id, unit, settings)
}

// SetBlockStorage adds the given block storage to the context.
func (w *ContextWrapper) SetBlockStorage(name, location string) {
	w.setStorage(name, location, storage.StorageKindBlock)
}
