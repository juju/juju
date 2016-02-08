// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jujuclient provides functionality to support
// connections to Juju such as controllers cache, accounts cache, etc.

package jujuclient

// TODO(anastasiamac 2016-02-08) all store methods should hold a file lock -
// If we don't lock, we're going to end up corrupting files.
type store struct {
}

// DefaultControllerStore returns files-based controller store
// rooted at JujuHome.
// TODO (anastasiamac 2016-02-08) aim to remove this
// and instead inject a Cache into commands that need to use it.
var DefaultControllerStore = func() (ControllerStore, error) {
	return &store{}, nil
}
