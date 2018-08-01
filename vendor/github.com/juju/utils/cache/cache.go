// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package cache provides a simple caching mechanism
// that limits the age of cache entries and tries to avoid large
// repopulation events by staggering refresh times.
package cache

import (
	"math/rand"
	"sync"
	"time"

	"gopkg.in/errgo.v1"
)

// entry holds a cache entry. The expire field
// holds the time after which the entry will be
// considered invalid.
type entry struct {
	value  interface{}
	expire time.Time
}

// Key represents a cache key. It must be a comparable type.
type Key interface{}

// Cache holds a time-limited set of values for arbitrary keys.
type Cache struct {
	maxAge time.Duration

	// mu guards the fields below it.
	mu sync.Mutex

	// expire holds when the cache is due to expire.
	expire time.Time

	// We hold two maps so that can avoid scanning through all the
	// items in the cache when the cache needs to be refreshed.
	// Instead, we move items from old to new when they're accessed
	// and throw away the old map at refresh time.
	old, new map[Key]entry

	inFlight map[Key]*fetchCall
}

// fetch represents an in-progress fetch call. If a cache Get request
// is made for an item that is currently being fetched, this will
// be used to avoid an extra call to the fetch function.
type fetchCall struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

// New returns a new Cache that will cache items for
// at most maxAge. If maxAge is zero, items will
// never be cached.
func New(maxAge time.Duration) *Cache {
	// The returned cache will have a zero-valued expire
	// time, so will expire immediately, causing the new
	// map to be created.
	return &Cache{
		maxAge:   maxAge,
		inFlight: make(map[Key]*fetchCall),
	}
}

// Len returns the total number of cached entries.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.old) + len(c.new)
}

// Evict removes the entry with the given key from the cache if present.
func (c *Cache) Evict(key Key) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.new, key)
	delete(c.old, key)
}

// EvictAll removes all entries from the cache.
func (c *Cache) EvictAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.new = make(map[Key]entry)
	c.old = nil
}

// Get returns the value for the given key, using fetch to fetch
// the value if it is not found in the cache.
// If fetch returns an error, the returned error from Get will have
// the same cause.
func (c *Cache) Get(key Key, fetch func() (interface{}, error)) (interface{}, error) {
	return c.getAtTime(key, fetch, time.Now())
}

// getAtTime is the internal version of Get, useful for testing; now represents the current
// time.
func (c *Cache) getAtTime(key Key, fetch func() (interface{}, error), now time.Time) (interface{}, error) {
	if val, ok := c.cachedValue(key, now); ok {
		return val, nil
	}
	c.mu.Lock()
	if f, ok := c.inFlight[key]; ok {
		// There's already an in-flight request for the key, so wait
		// for that to complete and use its results.
		c.mu.Unlock()
		f.wg.Wait()
		// The value will have been added to the cache by the first fetch,
		// so no need to add it here.
		if f.err == nil {
			return f.val, nil
		}
		return nil, errgo.Mask(f.err, errgo.Any)
	}
	var f fetchCall
	f.wg.Add(1)
	c.inFlight[key] = &f
	// Mark the request as done when we return, and after
	// the value has been added to the cache.
	defer f.wg.Done()

	// Fetch the data without the mutex held
	// so that one slow fetch doesn't hold up
	// all the other cache accesses.
	c.mu.Unlock()
	val, err := fetch()
	c.mu.Lock()
	defer c.mu.Unlock()

	// Set the result in the fetchCall so that other calls can see it.
	f.val, f.err = val, err
	if err == nil && c.maxAge >= 2*time.Nanosecond {
		// If maxAge is < 2ns then the expiry code will panic because the
		// actual expiry time will be maxAge - a random value in the
		// interval [0, maxAge/2). If maxAge is < 2ns then this requires
		// a random interval in [0, 0) which causes a panic.
		//
		// This value is so small that there's no need to cache anyway,
		// which makes tests more obviously deterministic when using
		// a zero expiry time.
		c.new[key] = entry{
			value:  val,
			expire: now.Add(c.maxAge - time.Duration(rand.Int63n(int64(c.maxAge/2)))),
		}
	}
	delete(c.inFlight, key)
	if err == nil {
		return f.val, nil
	}
	return nil, errgo.Mask(f.err, errgo.Any)
}

// cachedValue returns any cached value for the given key
// and whether it was found.
func (c *Cache) cachedValue(key Key, now time.Time) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now.After(c.expire) {
		c.old = c.new
		c.new = make(map[Key]entry)
		c.expire = now.Add(c.maxAge)
	}
	if e, ok := c.entry(c.new, key, now); ok {
		return e.value, true
	}
	if e, ok := c.entry(c.old, key, now); ok {
		// An old entry has been accessed; move it to the new
		// map so that we only use a single map access for
		// subsequent lookups. Note that because we use the same
		// duration for cache refresh (c.expire) as for max
		// entry age, this is strictly speaking unnecessary
		// because any entries in old will have expired by the
		// time it is dropped.
		c.new[key] = e
		delete(c.old, key)
		return e.value, true
	}
	return nil, false
}

// entry returns an entry from the map and whether it
// was found. If the entry has expired, it is deleted from the map.
func (c *Cache) entry(m map[Key]entry, key Key, now time.Time) (entry, bool) {
	e, ok := m[key]
	if !ok {
		return entry{}, false
	}
	if now.After(e.expire) {
		// Delete expired entries.
		delete(m, key)
		return entry{}, false
	}
	return e, true
}
