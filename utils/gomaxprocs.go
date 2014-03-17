// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"runtime"
	"sync"
)

var mu = sync.Mutex{}
var gomaxprocs = runtime.GOMAXPROCS
var numcpu = runtime.NumCPU
var enabledMultiCPUs = false

// EnableMultipleCPUs is called when we want to allow the system to have
// GOMAXPROCS>1. By default we only want to enable GOMAXPROCS>1 when running in
// the jujud machine agents that are serving the API. We don't want to set it
// in the test suite, etc.
// So first you call EnableMultipleCPUs() when we determine we are running from
// main(), and then later on you call UseMultipleCPUs() when we determine we
// are running a machine agent that is serving the API.
func EnableMultipleCPUs() {
	mu.Lock()
	defer mu.Unlock()
	// We check to see if GOMAXPROCS is set in the environment. If it is,
	// then we will just use the environment variable, and not override it
	// ourselves
	if os.Getenv("GOMAXPROCS") == "" {
		enabledMultiCPUs = true
	}
}


// UseMultipleCPUs is called when we have decided we want to set GOMAXPROCS to
// the current number of CPU cores. This will not override the environment
// variable if it is set.
func UseMultipleCPUs() {
	mu.Lock()
	defer mu.Unlock()
	if !enabledMultiCPUs {
		logger.Debugf("multiple CPUs not enabled, calling GOMAXPROCS(0)")
		gomaxprocs(0)
		return
	}
	n := numcpu()
	logger.Debugf("setting GOMAXPROCS to %d", n)
	gomaxprocs(n)
}
