// Copyright 2014 Cloudbase Solutions SRL
// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.
//
// +build !windows

package uptime

import (
	"syscall"
)

// Uptime returns the number of seconds since the system has booted
func Uptime() (int64, error) {
	info := &syscall.Sysinfo_t{}
	err := syscall.Sysinfo(info)
	if err != nil {
		return 0, err
	}
	return int64(info.Uptime), nil
}
