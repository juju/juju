// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// RelationHook holds the values for the hook context.
type RelationHook struct {
	HookRelation   jujuc.ContextRelation
	RemoteUnitName string
}

// Reset clears the RelationHook's data.
func (rh *RelationHook) Reset() {
	rh.HookRelation = nil
	rh.RemoteUnitName = ""
}

// ContextRelationHook is a test double for jujuc.RelationHookContext.
type ContextRelationHook struct {
	contextBase
	info *RelationHook
}

// HookRelation implements jujuc.RelationHookContext.
func (c *ContextRelationHook) HookRelation() (jujuc.ContextRelation, bool) {
	c.stub.AddCall("HookRelation")
	c.stub.NextErr()

	return c.info.HookRelation, c.info.HookRelation != nil
}

// RemoteUnitName implements jujuc.RelationHookContext.
func (c *ContextRelationHook) RemoteUnitName() (string, bool) {
	c.stub.AddCall("RemoteUnitName")
	c.stub.NextErr()

	return c.info.RemoteUnitName, c.info.RemoteUnitName != ""
}
