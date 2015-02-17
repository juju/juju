// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package upstart

import (
	"github.com/juju/juju/service/initsystems"
)

var (
	ConfDir = &confDir
)

func NewUpstart(initDir string, fops fileOperations, cmd cmdRunner) initsystems.InitSystem {
	return &upstart{
		name:    "upstart",
		initDir: initDir,
		fops:    fops,
		cmd:     cmd,
	}
}
