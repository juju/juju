// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

// OverrideCPUFuncs changes what functions will be used for runtime.GOMAXPROCS
// and runtime.NumCPU. The returned function will restore them to their
// original values.
func OverrideCPUFuncs(maxprocsfunc func(int) int, numCPUFunc func() int) func() {
	origGOMAXPROCS := gomaxprocs
	gomaxprocs = maxprocsfunc
	origNumCPU := numcpu
	numcpu = numCPUFunc
	return func() {
		gomaxprocs = origGOMAXPROCS
		numcpu = origNumCPU
	}
}
