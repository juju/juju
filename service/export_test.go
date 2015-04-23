// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

var (
	DiscoverInitSystem      = discoverInitSystem
	DiscoverLocalInitSystem = discoverLocalInitSystem
	NewShellSelectCommand   = newShellSelectCommand
)

func NewDiscoveryCheck(name string, running bool, failure error) discoveryCheck {
	return discoveryCheck{
		name: name,
		isRunning: func() (bool, error) {
			return running, failure
		},
	}
}

type patcher interface {
	PatchValue(interface{}, interface{})
}

func PatchDiscoveryFuncs(s patcher, checks ...discoveryCheck) {
	s.PatchValue(&discoveryFuncs, checks)
}
