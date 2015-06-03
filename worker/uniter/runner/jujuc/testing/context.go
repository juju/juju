// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
)

// ContextInfo holds the values for the hook context.
type ContextInfo struct {
	*Unit
	*Instance
	*NetworkInterface
	*Leadership
	*Storage
	*Components
	*Relations
	*RelationHook
	*Action
}

// NewContextInfo returns a new ContextInfo.
func NewContextInfo() *ContextInfo {
	return &ContextInfo{
		Unit:             &Unit{},
		Instance:         &Instance{},
		NetworkInterface: &NetworkInterface{},
		Leadership:       &Leadership{},
		Storage:          &Storage{},
		Components:       &Components{},
		Relations:        &Relations{},
		RelationHook:     &RelationHook{},
		Action:           &Action{},
	}
}

// Context is a test double for jujuc.Context.
type Context struct {
	ContextUnit
	ContextInstance
	ContextNetworking
	ContextLeader
	ContextMetrics
	ContextStorage
	ContextComponents
	ContextRelations
	ContextAction
}

// NewContext returns a new Context.
func NewContext(stub *testing.Stub, info *ContextInfo) *Context {
	if info == nil {
		info = NewContextInfo()
	}
	return &Context{
		ContextUnit:       ContextUnit{stub, info.Unit},
		ContextInstance:   ContextInstance{stub, info.Instance},
		ContextNetworking: ContextNetworking{stub, info.NetworkInterface},
		ContextLeader:     ContextLeader{stub, info.Leadership},
		ContextMetrics:    ContextMetrics{stub},
		ContextStorage:    ContextStorage{stub, info.Storage},
		ContextComponents: ContextComponents{stub, info.Components},
		ContextRelations:  ContextRelations{stub, info.Relations, info.RelationHook},
		ContextAction:     ContextAction{stub, info.Action},
	}
}
