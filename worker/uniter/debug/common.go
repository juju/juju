// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import (
	"fmt"
	"path/filepath"

	"launchpad.net/juju-core/state"
)

const defaultFlockDir = "/tmp"

type DebugHooksContext struct {
	Unit     string
	FlockDir string
}

func NewDebugHooksContext(unitName string) *DebugHooksContext {
	return &DebugHooksContext{Unit: unitName, FlockDir: defaultFlockDir}
}

func (c *DebugHooksContext) ClientFileLock() string {
	basename := fmt.Sprintf("juju-%s-debug-hooks", state.UnitTag(c.Unit))
	return filepath.Join(c.FlockDir, basename)
}

func (c *DebugHooksContext) ClientExitFileLock() string {
	return c.ClientFileLock() + "-exit"
}

func (c *DebugHooksContext) tmuxSessionName() string {
	return c.Unit
}
