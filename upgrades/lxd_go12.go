// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !go1.3

package upgrades

func updateLXDCloudCredentials(st StateBackend) error {
	// The LXD provider is compiled out when Juju is
	// built with Go 1.2 or earlier, so there is nothing
	// to do.
	return nil
}
