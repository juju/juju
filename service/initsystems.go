// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/service/initsystems"
	"github.com/juju/juju/service/initsystems/upstart"
	"github.com/juju/juju/service/initsystems/windows"
)

// These are the names of the juju-compatible init systems.
const (
	InitSystemWindows = "windows"
	InitSystemUpstart = "upstart"
)

var (
	linuxInitNames = map[string]string{
		"/sbin/init": InitSystemUpstart,
	}
)

func newInitSystem(name string) initsystems.InitSystem {
	switch name {
	case InitSystemWindows:
		return windows.NewInitSystem(name)
	case InitSystemUpstart:
		return upstart.NewInitSystem(name)
	}
	return nil
}
