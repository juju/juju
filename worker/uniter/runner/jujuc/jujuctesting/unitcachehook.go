// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"sync"
)

type UnitCache struct {
	values map[string]string
	mu     sync.Mutex
}

// ContextUnitCache is a test double for jujuc.unitCacheContext.
type ContextUnitCache struct {
	contextBase
	info *UnitCache
}

func (u *UnitCache) SetCache(newCache map[string]string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.values = newCache
}

// GetCache implements jujuc.unitCacheContext.
func (c *ContextUnitCache) GetCache() (map[string]string, error) {
	c.stub.AddCall("GetCache")
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

// Implements jujuc.HookContext.unitCacheContext, part of runner.Context.
func (c *ContextUnitCache) GetSingleCacheValue(key string) (string, error) {
	c.stub.AddCall("GetSingleCacheValue")
	c.info.mu.Lock()
	defer c.info.mu.Unlock()

	c.ensureValues()
	return c.info.values[key], nil
}

// GetCache implements jujuc.unitCacheContext.
func (c *ContextUnitCache) DeleteCacheValue(key string) error {
	c.stub.AddCall("DeleteCacheValue")
	_ = c.stub.NextErr()
	c.info.mu.Lock()
	defer c.info.mu.Unlock()
	c.ensureValues()

	delete(c.info.values, key)
	return nil
}

// GetCache implements jujuc.unitCacheContext.
func (c *ContextUnitCache) SetCacheValue(key string, value string) error {
	c.stub.AddCall("SetCacheValue")
	_ = c.stub.NextErr()
	c.info.mu.Lock()
	defer c.info.mu.Unlock()
	c.ensureValues()

	c.info.values[key] = value
	return nil
}

func (c *ContextUnitCache) ensureValues() {
	if c.info.values == nil {
		c.info.values = make(map[string]string)
	}
}
