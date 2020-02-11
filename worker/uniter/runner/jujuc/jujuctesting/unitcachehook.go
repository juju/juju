// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

type UnitCache struct {
}

// ContextUnitCache is a test double for jujuc.unitCacheContext.
type ContextUnitCache struct {
	contextBase
}

func (u *UnitCache) SetCache(map[string]string) {
}

// GetCache implements jujuc.unitCacheContext.
func (c *ContextUnitCache) GetCache() (map[string]string, error) {
	return nil, nil
}

// Implements jujuc.HookContext.unitCacheContext, part of runner.Context.
func (c *ContextUnitCache) GetSingleCacheValue(key string) (string, error) {
	return "", nil
}

// GetCache implements jujuc.unitCacheContext.
func (c *ContextUnitCache) DeleteCacheValue(key string) error {
	return nil
}

// GetCache implements jujuc.unitCacheContext.
func (c *ContextUnitCache) SetCacheValue(key string, value string) error {
	return nil
}
