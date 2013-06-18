// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

// SetContainerDir allows tests in other packages to override the
// containerDir.
func SetContainerDir(dir string) (old string) {
	old = containerDir
	containerDir = dir
	return
}

// SetLxcContainerDir allows tests in other packages to override the
// lxcContainerDir.
func SetLxcContainerDir(dir string) (old string) {
	old = lxcContainerDir
	lxcContainerDir = dir
	return
}

// SetRemovedContainerDir allows tests in other packages to override the
// removedContainerDir.
func SetRemovedContainerDir(dir string) (old string) {
	old = removedContainerDir
	removedContainerDir = dir
	return
}
