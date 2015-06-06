// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"

	"github.com/juju/juju/storage"
)

// ContextInfo holds the values for the hook context.
type ContextInfo struct {
	Unit
	Status
	Instance
	NetworkInterface
	Leadership
	Metrics
	Storage
	Relations
	RelationHook
	ActionHook
}

type contextBase struct {
	stub *testing.Stub
}

// Context is a test double for jujuc.Context.
type Context struct {
	ContextUnit
	ContextStatus
	ContextInstance
	ContextNetworking
	ContextLeader
	ContextMetrics
	ContextStorage
	ContextRelations
	ContextRelationHook
	ContextActionHook
}

func newContext(stub *testing.Stub, info *ContextInfo) *Context {
	var ctx Context
	ctx.ContextUnit.stub = stub
	ctx.ContextUnit.info = &info.Unit
	ctx.ContextStatus.stub = stub
	ctx.ContextStatus.info = &info.Status
	ctx.ContextInstance.stub = stub
	ctx.ContextInstance.info = &info.Instance
	ctx.ContextNetworking.stub = stub
	ctx.ContextNetworking.info = &info.NetworkInterface
	ctx.ContextLeader.stub = stub
	ctx.ContextLeader.info = &info.Leadership
	ctx.ContextMetrics.stub = stub
	ctx.ContextMetrics.info = &info.Metrics
	ctx.ContextStorage.stub = stub
	ctx.ContextStorage.info = &info.Storage
	ctx.ContextRelations.stub = stub
	ctx.ContextRelations.info = &info.Relations
	ctx.ContextRelationHook.stub = stub
	ctx.ContextRelationHook.info = &info.RelationHook
	ctx.ContextActionHook.stub = stub
	ctx.ContextActionHook.info = &info.ActionHook
	return &ctx
}

// ContextHelper provides testing helpers for a hook Context.
type ContextHelper struct {
	stub    *testing.Stub
	info    *ContextInfo
	context *Context
}

// NewContextHelper build a ContextHelper around the given info.
func NewContextHelper(stub *testing.Stub, info *ContextInfo) *ContextHelper {
	return &ContextHelper{
		stub: stub,
		info: info,
	}
}

func (ch *ContextHelper) init() {
	if ch.context != nil {
		return
	}
	if ch.stub == nil {
		ch.stub = &testing.Stub{}
	}
	if ch.info == nil {
		ch.info = &ContextInfo{}
	}
	ch.context = newContext(ch.stub, ch.info)
}

// Context returns the context wrapped by the helper.
func (ch *ContextHelper) Context() *Context {
	ch.init()
	return ch.context
}

// SetAsRelationHook updates the context to work as a relation hook context.
func (ch *ContextHelper) SetAsRelationHook(id int, remote string) {
	ch.init()
	ch.info.HookRelation = ch.info.Relations.Relations[id]
	ch.info.RemoteUnitName = remote
}

// SetAsActionHook updates the context to work as an action hook context.
func (ch *ContextHelper) SetAsActionHook() {
	ch.init()
	panic("not supported yet")
}

// SetRelation sets up the specified relation within the context.
func (ch *ContextHelper) SetRelation(id int, name, unit string, settings Settings) *Relation {
	ch.init()
	relation := ch.context.ContextRelations.setRelation(id, name)
	relation.SetRelated(unit, settings)
	return relation
}

// SetRelated adds the provided unit information to the relation.
func (ch *ContextHelper) SetRelated(id int, unit string, settings Settings) {
	ch.init()
	relation := ch.info.Relations.Relations[id].(*ContextRelation).info
	relation.SetRelated(unit, settings)
}

func (ch *ContextHelper) setStorage(name, location string, kind storage.StorageKind) {
	ch.init()
	ch.context.ContextStorage.setAttachment(name, location, kind)
}

// SetBlockStorage adds the given block storage to the context.
func (ch *ContextHelper) SetBlockStorage(name, location string) {
	ch.setStorage(name, location, storage.StorageKindBlock)
}
