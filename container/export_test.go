// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package container

func IsLocked(lock *Lock) bool {
	return lock.lockFile != nil
}
