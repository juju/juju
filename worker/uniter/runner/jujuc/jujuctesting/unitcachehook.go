// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"sync"

	"github.com/juju/errors"
)

type UnitCache struct {
	values map[string]string
	mu     *sync.Mutex
}

// ContextUnitCache is a test double for jujuc.unitCacheContext.
type ContextUnitCache struct {
	contextBase
	info *UnitCache
}

// GetCacheValue implements jujuc.unitCacheContext.
func (c *ContextUnitCache) GetCacheValue(key string) (string, error) {
	c.stub.AddCall("GetCacheValue")
	_ = c.stub.NextErr()
	c.info.mu.Lock()
	defer c.info.mu.Unlock()
	c.ensureValues()
	value, ok := c.info.values[key]
	if !ok {
		return "", errors.NotFoundf("%q", key)
	}
	return value, nil
}

// GetCacheValue implements jujuc.unitCacheContext.
func (c *ContextUnitCache) DeleteCacheValue(key string) error {
	c.stub.AddCall("DeleteCacheValue")
	_ = c.stub.NextErr()
	c.info.mu.Lock()
	defer c.info.mu.Unlock()
	c.ensureValues()

	delete(c.info.values, key)
	return nil
}

// GetCacheValue implements jujuc.unitCacheContext.
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
