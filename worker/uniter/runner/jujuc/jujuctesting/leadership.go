// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

// Leadership holds the values for the hook context.
type Leadership struct {
}

// ContextLeader is a test double for jujuc.ContextLeader.
type ContextLeader struct {
	contextBase
}

// IsLeader implements jujuc.ContextLeader.
func (c *ContextLeader) IsLeader() (bool, error) {
	return false, nil
}

// LeaderSettings implements jujuc.ContextLeader.
func (c *ContextLeader) LeaderSettings() (map[string]string, error) {
	return nil, nil
}

// WriteLeaderSettings implements jujuc.ContextLeader.
func (c *ContextLeader) WriteLeaderSettings(settings map[string]string) error {
	return nil
}
