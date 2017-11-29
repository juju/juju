// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hooktesting

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/common/hookcommands"
)

// RelationHook holds the values for the hook context.
type RelationHook struct {
	HookRelation   hookcommands.ContextRelation
	RemoteUnitName string
}

// Reset clears the RelationHook's data.
func (rh *RelationHook) Reset() {
	rh.HookRelation = nil
	rh.RemoteUnitName = ""
}

// ContextRelationHook is a test double for hooks.RelationHookContext.
type ContextRelationHook struct {
	contextBase
	info *RelationHook
}

// HookRelation implements hooks.RelationHookContext.
func (c *ContextRelationHook) HookRelation() (hookcommands.ContextRelation, error) {
	c.stub.AddCall("HookRelation")
	var err error
	if c.info.HookRelation == nil {
		err = errors.NotFoundf("hook relation")
	}

	return c.info.HookRelation, err
}

// RemoteUnitName implements hooks.RelationHookContext.
func (c *ContextRelationHook) RemoteUnitName() (string, error) {
	c.stub.AddCall("RemoteUnitName")
	c.stub.NextErr()
	var err error
	if c.info.RemoteUnitName == "" {
		err = errors.NotFoundf("remote unit")
	}

	return c.info.RemoteUnitName, err
}
