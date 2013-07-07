// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"io"

	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

type DiskManager struct {
	dataDir string
}

func NewDiskManager(dataDir string) *DiskManager {
	return &DiskManager{dataDir: dataDir}
}

var _ ToolsManager = (*DiskManager)(nil)

// For now, everything is just proxied from environs/agent. But really tool
// handling should be independent of anything in environs.

func (d *DiskManager) ReadTools(vers version.Binary) (*Tools, error) {
	stTools, err := agent.ReadTools(d.dataDir, vers)
	return (*Tools)(stTools), err
}

func (d *DiskManager) UnpackTools(tools *Tools, r io.Reader) error {
	return agent.UnpackTools(d.dataDir, (*state.Tools)(tools), r)
}

func (d *DiskManager) SharedToolsDir(vers version.Binary) string {
	return agent.SharedToolsDir(d.dataDir, vers)
}
