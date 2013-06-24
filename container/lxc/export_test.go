// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

func SetContainerDir(dir string) (old string) {
	old, containerDir = containerDir, dir
	return
}

func SetLxcContainerDir(dir string) (old string) {
	old, lxcContainerDir = lxcContainerDir, dir
	return
}

func SetRemovedContainerDir(dir string) (old string) {
	old, removedContainerDir = removedContainerDir, dir
	return
}
