// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

// RunCleanUps runs the functions provided in the cleanUps slice in reverse
// order. This is a utility function for Kubernetes to help remove resources
// created when there is an error
func RunCleanUps(cleanUps []func()) {
	for j := len(cleanUps) - 1; j >= 0; j-- {
		cleanUps[j]()
	}
}
