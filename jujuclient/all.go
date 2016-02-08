// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jujuclient provides functionality to support
// connections to Juju such as controllers cache, accounts cache, etc.

package jujuclient

type store struct {
}

// DefaultControllerStore returns files-based controller store
// rooted at JujuHome.
var DefaultControllerStore = func() (ControllerStore, error) {
	return &store{}, nil
}
