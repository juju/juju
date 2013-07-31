// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import (
	"fmt"
	"path/filepath"

	"launchpad.net/juju-core/names"
)

const defaultFlockDir = "/tmp"

type HooksContext struct {
	Unit     string
	FlockDir string
}

func NewHooksContext(unitName string) *HooksContext {
	return &HooksContext{Unit: unitName, FlockDir: defaultFlockDir}
}

func (c *HooksContext) ClientFileLock() string {
	basename := fmt.Sprintf("juju-%s-debug-hooks", names.UnitTag(c.Unit))
	return filepath.Join(c.FlockDir, basename)
}

func (c *HooksContext) ClientExitFileLock() string {
	return c.ClientFileLock() + "-exit"
}

func (c *HooksContext) tmuxSessionName() string {
	return c.Unit
}
