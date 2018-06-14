// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"errors"

	"github.com/juju/juju/container"
	lxdclient "github.com/lxc/lxd/client"
)

var (
	NewNICDevice             = newNICDevice
	NetworkDevicesFromConfig = networkDevicesFromConfig
	CheckBridgeConfigFile    = checkBridgeConfigFile
	SeriesRemoteAliases      = seriesRemoteAliases
	ErrIPV6NotSupported      = errIPV6NotSupported
)

type patcher interface {
	PatchValue(interface{}, interface{})
}

// PatchConnectRemote ensures that the ConnectImageRemote function always returns
// the supplied (mock) image server.
func PatchConnectRemote(patcher patcher, remotes map[string]lxdclient.ImageServer) {
	patcher.PatchValue(&ConnectImageRemote, func(remote RemoteServer) (lxdclient.ImageServer, error) {
		if svr, ok := remotes[remote.Name]; ok {
			return svr, nil
		}
		return nil, errors.New("unrecognised remote server")
	})
}

func GetImageSources(mgr container.Manager) ([]RemoteServer, error) {
	return mgr.(*containerManager).getImageSources()
}

func VerifyNICsWithConfigFile(svr *Server, nics map[string]device, reader func(string) ([]byte, error)) error {
	return svr.verifyNICsWithConfigFile(nics, reader)
}
