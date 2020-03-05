// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

// Set of helper methods for working with k8s objects

// Returns a ptr for the supplied 32 bit integer
func int32Ptr(i int32) *int32 {
	return &i
}
