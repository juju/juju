// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jujuclient provides functionality to support
// connections to Juju such as controllers cahce, accounts cache, etc.

package jujuclient

type cacheAll struct {
	controllersFile
}

var Default = func() (Cache, error) {
	return &cacheAll{}, nil
}
