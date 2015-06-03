// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
)

// Action holds the values for the hook context.
type Action struct {
	ActionParams map[string]interface{}
}

// ContextAction is a test double for jujuc.ActionHookContext.
type ContextAction struct {
	Stub *testing.Stub
	Info *Action
}

// ActionParams implements jujuc.ActionHookContext.
func (c *ContextAction) ActionParams() (map[string]interface{}, error) {
	c.Stub.AddCall("ActionParams")
	if err := c.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	if c.Info.ActionParams == nil {
		return nil, errors.Errorf("not running an action")
	}
	return c.Info.ActionParams, nil
}

// UpdateActionResults implements jujuc.ActionHookContext.
func (c *ContextAction) UpdateActionResults(keys []string, value string) error {
	c.Stub.AddCall("UpdateActionResults", keys, value)
	if err := c.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if c.Info.ActionParams == nil {
		return errors.Errorf("not running an action")
	}
	return nil
}

// SetActionMessage implements jujuc.ActionHookContext.
func (c *ContextAction) SetActionMessage(message string) error {
	c.Stub.AddCall("SetActionMessage", message)
	if err := c.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if c.Info.ActionParams == nil {
		return errors.Errorf("not running an action")
	}
	return nil
}

// SetActionFailed implements jujuc.ActionHookContext.
func (c *ContextAction) SetActionFailed() error {
	c.Stub.AddCall("SetActionFailed")
	if err := c.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if c.Info.ActionParams == nil {
		return errors.Errorf("not running an action")
	}
	return nil
}
