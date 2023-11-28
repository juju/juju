// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

// TestLXDManager for use in worker/provisioner tests
type TestLXDManager interface {
	Manager
	LXDProfileManager
	LXDProfileNameRetriever
}
