// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

func IsMultipleCPUsEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabledMultiCPUs
}
