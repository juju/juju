// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"sync"
	"time"
)

// ResourceDownloadLock is used to limit the number of concurrent
// resource downloads and per application. The total number of
// downloads across all applications cannot exceed the global limit.
type ResourceDownloadLock interface {
	// Acquire grabs the lock for a given application so long as the
	// per application limit is not exceeded and total across all
	// applications does not exceed the global limit.
	Acquire(appName string)

	// Release releases the lock for the given application.
	Release(appName string)
}

// NewResourceDownloadLimiter creates a new resource download limiter.
func NewResourceDownloadLimiter(globalLimit, applicationLimit int) *resourceDownloadLimiter {
	limiter := &resourceDownloadLimiter{
		applicationLimit: applicationLimit,
		applicationLocks: make(map[string]chan struct{}),
	}
	if globalLimit > 0 {
		limiter.globalLock = make(chan struct{}, globalLimit)
	}
	return limiter
}

type resourceDownloadLimiter struct {
	globalLock chan struct{}

	mu               sync.Mutex
	applicationLimit int
	applicationLocks map[string]chan struct{}
}

// Acquire implements ResourceDownloadLock.
func (r *resourceDownloadLimiter) Acquire(appName string) {
	if r.globalLock != nil {
		start := time.Now()
		r.globalLock <- struct{}{}
		logger.Debugf("acquire global resource download lock for %q, took %dms", appName, time.Now().Sub(start)/time.Millisecond)
	}
	if r.applicationLimit <= 0 {
		return
	}

	r.mu.Lock()
	lock, ok := r.applicationLocks[appName]
	if !ok {
		lock = make(chan struct{}, r.applicationLimit)
		r.applicationLocks[appName] = lock
	}
	r.mu.Unlock()
	start := time.Now()
	lock <- struct{}{}
	logger.Debugf("acquire app resource download lock for %q, took %dms", appName, time.Now().Sub(start)/time.Millisecond)
}

// Release implements ResourceDownloadLock.
func (r *resourceDownloadLimiter) Release(appName string) {
	if r.globalLock != nil {
		<-r.globalLock
	}
	if r.applicationLimit <= 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	lock, ok := r.applicationLocks[appName]
	if !ok {
		return
	}
	<-lock
	if len(lock) == 0 {
		delete(r.applicationLocks, appName)
	}
}
