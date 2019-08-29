// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"errors"

	"github.com/juju/clock"
	lxdclient "github.com/lxc/lxd/client"

	"github.com/juju/juju/container"
	"github.com/juju/juju/core/network"
)

var (
	NewNICDevice          = newNICDevice
	CheckBridgeConfigFile = checkBridgeConfigFile
	SeriesRemoteAliases   = seriesRemoteAliases
	ErrIPV6NotSupported   = errIPV6NotSupported
)

type patcher interface {
	PatchValue(interface{}, interface{})
}

// PatchConnectRemote ensures that the ConnectImageRemote function always returns
// the supplied (mock) image server.
func PatchConnectRemote(patcher patcher, remotes map[string]lxdclient.ImageServer) {
	patcher.PatchValue(&ConnectImageRemote, func(spec ServerSpec) (lxdclient.ImageServer, error) {
		if svr, ok := remotes[spec.Name]; ok {
			return svr, nil
		}
		return nil, errors.New("unrecognized remote server")
	})
}

func PatchGenerateVirtualMACAddress(patcher patcher) {
	patcher.PatchValue(&network.GenerateVirtualMACAddress, func() string {
		return "00:16:3e:00:00:00"
	})
}

func PatchLXDViaSnap(patcher patcher, isSnap bool) {
	patcher.PatchValue(&lxdViaSnap, func() bool { return isSnap })
}

func PatchHostSeries(patcher patcher, series string) {
	patcher.PatchValue(&hostSeries, func() (string, error) { return series, nil })
}

func GetImageSources(mgr container.Manager) ([]ServerSpec, error) {
	return mgr.(*containerManager).getImageSources()
}

func VerifyNICsWithConfigFile(svr *Server, nics map[string]device, reader func(string) ([]byte, error)) error {
	return svr.verifyNICsWithConfigFile(nics, reader)
}

func NetworkDevicesFromConfig(mgr container.Manager, netConfig *container.NetworkConfig) (
	map[string]device, []string, error,
) {
	cMgr := mgr.(*containerManager)
	return cMgr.networkDevicesFromConfig(netConfig)
}

func NewTestingServer(svr lxdclient.ContainerServer, clock clock.Clock) (*Server, error) {
	server, err := NewServer(svr)
	if err != nil {
		return nil, err
	}
	server.clock = clock
	return server, nil
}
