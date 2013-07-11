// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"path/filepath"
	"time"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/fslock"
)

// Functions in this package are here to help us with post-upgrade installing,
// etc. It is used to validate the system status after we have upgraded.

var validationLogger = loggo.GetLogger("juju.jujud.validation")

var lockTimeout = 1 * time.Minute

// getUniterLock grabs the "uniter-hook-execution" lock and holds it, or an error
func getUniterLock(dataDir, message string) (*fslock.Lock, error) {
	// Taken from worker/uniter/uniter.go setupLocks()
	lockDir := filepath.Join(dataDir, "locks")
	hookLock, err := fslock.NewLock(lockDir, "uniter-hook-execution")
	if err != nil {
		// Note: Should we try to do the install anyway, just without the lock?
		return nil, err
	}
	err = hookLock.LockWithTimeout(lockTimeout, message)
	if err != nil {
		return nil, err
	}
	return hookLock, nil
}

// EnsureWeHaveLXC checks if we have lxc installed, and installs it if we
// don't. Juju 1.11 added the ability to deploy into LXC containers, and uses
// functionality from the lxc package in order to do so. Juju 1.10 did not
// install lxc, so we ensure it is installed.
// See http://bugs.launchpad.net/bug/1199913
// dataDir is the root location where data files are put. It is used to grab
// the uniter-hook-execution lock so that we don't try run to apt-get at the
// same time that a hook might want to run it.
func EnsureWeHaveLXC(dataDir string) error {
	manager := lxc.NewContainerManager("lxc-test")
	if _, err := manager.ListContainers(); err == nil {
		validationLogger.Debugf("found lxc, not installing")
		// We already have it, nothing more to do
		return nil
	}
	validationLogger.Debugf("got error looking for lxc, attempting to install")
	if dataDir != "" {
		lock, err := getUniterLock(dataDir, "apt-get install lxc for juju 1.11 upgrade")
		if err == nil {
			defer lock.Unlock()
		}
		// If we got an error trying to acquire the lock, we try to install
		// lxc anyway. Worst case the install will fail, which is where we
		// are already
	}
	// TODO: This is not platform independent. If jujud is running on
	//       something other than a debian-based Linux, and we are missing
	//       lxc, this call will always fail. However, in juju 1.11+ we
	//       install lxc via cloud-init or whatever bootstrap code we use.
	//       So this is really only upgrade compatibility and juju 1.10
	//       only supports debian-based anyway
	return utils.AptGetInstall("lxc")
}
