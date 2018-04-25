// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/juju/container"
	lxdclient "github.com/lxc/lxd/client"
)

var (
	OsStat = &osStat
	//LxdConnectPublicLXD     = &lxdConnectPublicLXD
	//LxdConnectSimpleStreams = &lxdConnectSimpleStreams
	NICDevice       = nicDevice
	NetworkDevices  = networkDevices
	GetImageSources = func(mgr container.Manager) ([]RemoteServer, error) {
		return mgr.(*containerManager).getImageSources()
	}
)

type patcher interface {
	PatchValue(interface{}, interface{})
}

// PatchConnectRemote ensures that the ConnectImageRemote function always returns
// the supplied (mock) image server.
func PatchConnectRemote(patcher patcher, svr lxdclient.ImageServer) {
	patcher.PatchValue(&ConnectImageRemote, func(_ RemoteServer) (lxdclient.ImageServer, error) { return svr, nil })
}
