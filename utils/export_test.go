// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

func OverrideGOMAXPROCSFuncs(newGOMAXPROCS func(int) int, newNumCPU func() int) (cleanup func()) {
	mu.Lock()
	defer mu.Unlock()
	origGOMAXPROCS := gomaxprocs
	logger.Debugf("setting GOMAXPROCS to %#v", newGOMAXPROCS)
	gomaxprocs = newGOMAXPROCS
	origNumCPU := numcpu
	numcpu = newNumCPU
	return func() {
		mu.Lock()
		defer mu.Unlock()
		gomaxprocs = origGOMAXPROCS
		numcpu = origNumCPU
		enabledMultiCPUs = false
	}
}

func IsMultipleCPUsEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabledMultiCPUs
}
