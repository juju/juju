// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"fmt"

	"github.com/juju/loggo"
	"github.com/juju/testing"
)

// ContextInfo holds the values for the hook context.
type ContextInfo struct {
	Unit
	UnitCharmState
	Status
	Instance
	NetworkInterface
	Leadership
	Metrics
	Storage
	Relations
	RelationHook
	ActionHook
	Version
	WorkloadHook
}

// Context returns a Context that wraps the info.
func (info *ContextInfo) Context(stub *testing.Stub) *Context {
	return NewContext(stub, info)
}

// SetAsRelationHook updates the context to work as a relation hook context.
func (info *ContextInfo) SetAsRelationHook(id int, remote string) {
	relation, ok := info.Relations.Relations[id]
	if !ok {
		panic(fmt.Sprintf("relation #%d not added yet", id))
	}
	info.HookRelation = relation
	info.RemoteUnitName = remote
}

// SetRemoteApplicationName defines the remote application
func (info *ContextInfo) SetRemoteApplicationName(remote string) {
	info.RemoteApplicationName = remote
}

// SetAsActionHook updates the context to work as an action hook context.
func (info *ContextInfo) SetAsActionHook() {
	panic("not supported yet")
}

type contextBase struct {
	stub *testing.Stub
}

// Context is a test double for jujuc.Context.
type Context struct {
	ContextUnit
	ContextUnitCharmState
	ContextStatus
	ContextInstance
	ContextNetworking
	ContextLeader
	ContextMetrics
	ContextStorage
	ContextResources
	ContextPayloads
	ContextRelations
	ContextRelationHook
	ContextActionHook
	ContextVersion
	ContextWorkloadHook
	ContextSecrets
}

// NewContext builds a jujuc.Context test double.
func NewContext(stub *testing.Stub, info *ContextInfo) *Context {
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
	ctx.ContextResources.stub = stub
	ctx.ContextPayloads.stub = stub
	ctx.ContextRelations.stub = stub
	ctx.ContextRelations.info = &info.Relations
	ctx.ContextRelationHook.stub = stub
	ctx.ContextRelationHook.info = &info.RelationHook
	ctx.ContextActionHook.stub = stub
	ctx.ContextActionHook.info = &info.ActionHook
	ctx.ContextVersion.stub = stub
	ctx.ContextVersion.info = &info.Version
	ctx.ContextUnitCharmState.stub = stub
	ctx.ContextUnitCharmState.info = &info.UnitCharmState
	ctx.ContextWorkloadHook.stub = stub
	ctx.ContextWorkloadHook.info = &info.WorkloadHook
	ctx.ContextSecrets.stub = stub
	return &ctx
}

func (c *Context) GetLogger(module string) loggo.Logger {
	return loggo.GetLogger(module)
}
