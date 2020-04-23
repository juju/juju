// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3, !linux

package lxd

import (
	"github.com/juju/proxy"
	"github.com/juju/utils/series"

	"github.com/juju/juju/container"
)

type containerInitialiser struct {
	series string
}

// containerInitialiser implements container.Initialiser.
var _ container.Initialiser = (*containerInitialiser)(nil)

// NewContainerInitialiser  - on anything but Linux this is a NOP
func NewContainerInitialiser(string) container.Initialiser {
	return &containerInitialiser{}
}

// Initialise - on anything but Linux this is a NOP
func (ci *containerInitialiser) Initialise() error {
	return nil
}

// ConfigureLXDProxies - on anything but Linux this is a NOP
func ConfigureLXDProxies(proxies proxy.Settings) error {
	return nil
}

// lxdViaSnap interrogates the location of the Snap LXD socket in order
// to determine if LXD is being provided via that method.
// Always return false for other arch's
var lxdViaSnap = func() bool {
	return false
}

// hostSeries is only created because export_test wants to be able to patch it.
// Patching it has no effect on non-linux
var hostSeries = series.HostSeries
