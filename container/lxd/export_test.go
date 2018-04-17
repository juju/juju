// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/juju/container"
	"github.com/juju/juju/tools/lxdtools"
)

var (
	NICDevice       = nicDevice
	NetworkDevices  = networkDevices
	GetImageSources = func(mgr container.Manager) ([]lxdtools.RemoteServer, error) {
		return mgr.(*containerManager).getImageSources()
	}
)
