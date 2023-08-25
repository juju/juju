// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows

package systemd

import (
	"github.com/coreos/go-systemd/v22/util"
)

// IsRunning returns whether or not systemd is the local init system.
func IsRunning() bool {
	return util.IsRunningSystemd()
}
