// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/juju/container"
	"github.com/juju/juju/tools/lxdclient"
)

var (
	NewNICDevice             = newNICDevice
	NetworkDevicesFromConfig = networkDevicesFromConfig
	EnsureIPv4               = ensureIPv4
	GetImageSources          = func(mgr container.Manager) ([]lxdclient.Remote, error) {
		return mgr.(*containerManager).getImageSources()
	}
)
