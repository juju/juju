// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3, !linux

package lxd

import (
	"github.com/juju/utils/proxy"

	"github.com/juju/juju/container"
)

type containerInitialiser struct {
	series string
}

// containerInitialiser implements container.Initialiser.
var _ container.Initialiser = (*containerInitialiser)(nil)

// NewContainerInitialiser  - on anything but Linux this is a NOP
func NewContainerInitialiser(series string) container.Initialiser {
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
