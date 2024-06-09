// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"context"
	"sync"
)

type UnitCharmState struct {
	values map[string]string
	mu     sync.Mutex
}

// ContextUnitCharmState is a test double for jujuc.unitCharmStateContext.
type ContextUnitCharmState struct {
	contextBase
	info *UnitCharmState
}

func (u *UnitCharmState) SetCharmState(newCharmState map[string]string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.values = newCharmState
}

// GetCharmState implements jujuc.unitCharmStateContext.
func (c *ContextUnitCharmState) GetCharmState(_ context.Context) (map[string]string, error) {
	c.stub.AddCall("GetCharmState")
	_ = c.stub.NextErr()
	c.info.mu.Lock()
	defer c.info.mu.Unlock()
	c.ensureValues()
	if len(c.info.values) == 0 {
		return nil, nil
	}

	retVal := make(map[string]string, len(c.info.values))
	for k, v := range c.info.values {
		retVal[k] = v
	}
	return retVal, nil
}

// GetCharmStateValue implements jujuc.unitCharmStateContext.
func (c *ContextUnitCharmState) GetCharmStateValue(_ context.Context, key string) (string, error) {
	c.stub.AddCall("GetSCharmStateValue")
	c.info.mu.Lock()
	defer c.info.mu.Unlock()

	c.ensureValues()
	return c.info.values[key], nil
}

// DeleteCharmStateValue implements jujuc.unitCharmStateContext.
func (c *ContextUnitCharmState) DeleteCharmStateValue(_ context.Context, key string) error {
	c.stub.AddCall("DeleteCharmStateValue")
	_ = c.stub.NextErr()
	c.info.mu.Lock()
	defer c.info.mu.Unlock()
	c.ensureValues()

	delete(c.info.values, key)
	return nil
}

// SetCharmStateValue implements jujuc.unitCharmStateContext.
func (c *ContextUnitCharmState) SetCharmStateValue(_ context.Context, key string, value string) error {
	c.stub.AddCall("SetCharmStateValue")
	_ = c.stub.NextErr()
	c.info.mu.Lock()
	defer c.info.mu.Unlock()
	c.ensureValues()

	c.info.values[key] = value
	return nil
}

func (c *ContextUnitCharmState) ensureValues() {
	if c.info.values == nil {
		c.info.values = make(map[string]string)
	}
}
