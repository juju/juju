// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

var (
	// Variable is the address so we can PatchValue
	CheckIfRoot = &checkIfRoot

	// function exports for tests
	RunAsRoot       = runAsRoot
	JujuLocalPlugin = jujuLocalPlugin
)
