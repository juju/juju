// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

// RelationHook holds the values for the hook context.
type RelationHook struct {
	HookRelation          jujuc.ContextRelation
	RemoteUnitName        string
	RemoteApplicationName string
}

// Reset clears the RelationHook's data.
func (rh *RelationHook) Reset() {
	rh.HookRelation = nil
	rh.RemoteUnitName = ""
	rh.RemoteApplicationName = ""
}

// ContextRelationHook is a test double for jujuc.RelationHookContext.
type ContextRelationHook struct {
	contextBase
	info *RelationHook
}

// HookRelation implements jujuc.RelationHookContext.
func (c *ContextRelationHook) HookRelation() (jujuc.ContextRelation, error) {
	c.stub.AddCall("HookRelation")
	var err error
	if c.info.HookRelation == nil {
		err = errors.NotFoundf("hook relation")
	}

	return c.info.HookRelation, err
}

// RemoteUnitName implements jujuc.RelationHookContext.
func (c *ContextRelationHook) RemoteUnitName() (string, error) {
	c.stub.AddCall("RemoteUnitName")
	_ = c.stub.NextErr()
	var err error
	if c.info.RemoteUnitName == "" {
		err = errors.NotFoundf("remote unit")
	}

	return c.info.RemoteUnitName, err
}

// RemoteApplicationName implements jujuc.RelationHookContext.
func (c *ContextRelationHook) RemoteApplicationName() (string, error) {
	c.stub.AddCall("RemoteApplicationName")
	_ = c.stub.NextErr()
	var err error
	if c.info.RemoteApplicationName == "" {
		err = errors.NotFoundf("saas application")
	}

	return c.info.RemoteApplicationName, err
}
