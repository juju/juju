// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

var (
	NetworkInterfacesFile = &networkInterfacesFile
	CloudInitUserData     = cloudInitUserData
)

// IsLocked is used just to see if the local lock instance is locked, and
// is only required for use in tests.
func IsLocked(lock *Lock) bool {
	return lock.lockFile != nil
}
