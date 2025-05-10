// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"
)

// ResourceDownloadLock is used to limit the number of concurrent
// resource downloads and per application. The total number of
// downloads across all applications cannot exceed the global limit.
type ResourceDownloadLock interface {
	// Acquire grabs the lock for a given application so long as the
	// per application limit is not exceeded and total across all
	// applications does not exceed the global limit.
	Acquire(ctx context.Context, appName string) error

	// Release releases the lock for the given application.
	Release(appName string)
}

type resourceDownloadLimiter struct {
	globalLock *semaphore.Weighted

	mu               sync.Mutex
	applicationLimit int64
	applicationLocks map[string]appLock
}

// NewResourceDownloadLimiter creates a new resource download limiter.
func NewResourceDownloadLimiter(globalLimit, applicationLimit int) (*resourceDownloadLimiter, error) {
	if globalLimit < 0 || applicationLimit < 0 {
		return nil, fmt.Errorf("resource download limits must be non-negative")
	}
	limiter := &resourceDownloadLimiter{
		applicationLimit: int64(applicationLimit),
		applicationLocks: make(map[string]appLock),
	}
	if globalLimit > 0 {
		limiter.globalLock = semaphore.NewWeighted(int64(globalLimit))
	}
	return limiter, nil
}

// Acquire grabs the lock for a given application so long as the per application
// limit is not exceeded and total across all applications does not exceed the
// global limit.
func (r *resourceDownloadLimiter) Acquire(ctx context.Context, appName string) error {
	if r.globalLock != nil {
		start := time.Now()
		if err := r.globalLock.Acquire(ctx, 1); err != nil {
			return err
		}
		resourceLogger.Debugf(ctx, "acquire global resource download lock for %q, took %dms", appName, time.Now().Sub(start)/time.Millisecond)
	}
	if r.applicationLimit <= 0 {
		return nil
	}

	r.mu.Lock()
	lock, ok := r.applicationLocks[appName]
	if !ok {
		lock = appLock{
			lock: semaphore.NewWeighted(r.applicationLimit),
			size: 0,
		}
		r.applicationLocks[appName] = lock
	}
	r.mu.Unlock()

	start := time.Now()
	if err := lock.Acquire(ctx); err != nil {
		return err
	}
	resourceLogger.Debugf(ctx, "acquire app resource download lock for %q, took %dms", appName, time.Now().Sub(start)/time.Millisecond)
	return nil
}

// Release releases the lock for the given application.
func (r *resourceDownloadLimiter) Release(appName string) {
	if r.globalLock != nil {
		r.globalLock.Release(1)
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
	lock.Release()

	if lock.Size() == 0 {
		delete(r.applicationLocks, appName)
	}
}

type appLock struct {
	lock *semaphore.Weighted
	size int64
}

// Acquire grabs the lock for a given application so long as the
// per-application limit is not exceeded and total across all
// applications does not exceed the global limit.
func (a *appLock) Acquire(ctx context.Context) error {
	if err := a.lock.Acquire(ctx, 1); err != nil {
		return err
	}
	atomic.AddInt64(&a.size, 1)
	return nil
}

// Release releases the lock for the given application.
func (a *appLock) Release() {
	a.lock.Release(1)
	atomic.AddInt64(&a.size, -1)
}

// Size returns the current size of the lock.
func (a *appLock) Size() int64 {
	return atomic.LoadInt64(&a.size)
}

// noopDownloadResourceLocker is a no-op download resource locker.
type noopDownloadResourceLocker struct{}

// Acquire grabs the lock for a given application so long as the
// per-application limit is not exceeded and total across all
// applications does not exceed the global limit.
func (noopDownloadResourceLocker) Acquire(context.Context, string) error { return nil }

// Release releases the lock for the given application.
func (noopDownloadResourceLocker) Release(string) {}
