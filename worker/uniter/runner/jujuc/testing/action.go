// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
)

// ActionHook holds the values for the hook context.
type ActionHook struct {
	ActionParams map[string]interface{}
}

// ContextActionHook is a test double for jujuc.ActionHookContext.
type ContextActionHook struct {
	contextBase
	info *ActionHook
}

// ActionParams implements jujuc.ActionHookContext.
func (c *ContextActionHook) ActionParams() (map[string]interface{}, error) {
	c.stub.AddCall("ActionParams")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	if c.info.ActionParams == nil {
		return nil, errors.Errorf("not running an action")
	}
	return c.info.ActionParams, nil
}

// UpdateActionResults implements jujuc.ActionHookContext.
func (c *ContextActionHook) UpdateActionResults(keys []string, value string) error {
	c.stub.AddCall("UpdateActionResults", keys, value)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if c.info.ActionParams == nil {
		return errors.Errorf("not running an action")
	}
	return nil
}

// SetActionMessage implements jujuc.ActionHookContext.
func (c *ContextActionHook) SetActionMessage(message string) error {
	c.stub.AddCall("SetActionMessage", message)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if c.info.ActionParams == nil {
		return errors.Errorf("not running an action")
	}
	return nil
}

// SetActionFailed implements jujuc.ActionHookContext.
func (c *ContextActionHook) SetActionFailed() error {
	c.stub.AddCall("SetActionFailed")
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if c.info.ActionParams == nil {
		return errors.Errorf("not running an action")
	}
	return nil
}
