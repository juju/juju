// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runnertesting

import (
	"path/filepath"

	gc "gopkg.in/check.v1"
)

type fops interface {
	// MkDir provides the functionality of gc.C.MkDir().
	MkDir() string
}

// MockPaths implements Paths for tests that do touch the filesystem.
type MockPaths struct {
	tools         string
	charm         string
	socket        string
	metricsspool  string
	componentDirs map[string]string
	fops          fops
}

func NewMockPaths(c *gc.C) MockPaths {
	return MockPaths{
		tools:         c.MkDir(),
		charm:         c.MkDir(),
		socket:        filepath.Join(c.MkDir(), "test.sock"),
		metricsspool:  c.MkDir(),
		componentDirs: make(map[string]string),
		fops:          c,
	}
}

func (p MockPaths) GetMetricsSpoolDir() string {
	return p.metricsspool
}

func (p MockPaths) GetToolsDir() string {
	return p.tools
}

func (p MockPaths) GetCharmDir() string {
	return p.charm
}

func (p MockPaths) GetHookCommandSocket() string {
	return p.socket
}
