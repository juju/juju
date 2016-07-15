// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache // import "gopkg.in/juju/charmstore.v5-unstable/internal/cache"

import (
	"math/rand"
	"sync"
	"time"

	"gopkg.in/errgo.v1"
)

type entry struct {
	value  interface{}
	expire time.Time
}

// Cache holds a time-limited cache of values for string keys.
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
	old, new map[string]entry
}

// New returns a new Cache that will cache items for
// at most maxAge.
func New(maxAge time.Duration) *Cache {
	// A maxAge is < 2ns then the expiry code will panic because the
	// actual expiry time will be maxAge - a random value in the
	// interval [0. maxAge/2). If maxAge is < 2ns then this requires
	// a random interval in [0, 0) which causes a panic.
	if maxAge < 2*time.Nanosecond {
		maxAge = 2 * time.Nanosecond
	}
	// The returned cache will have a zero-valued expire
	// time, so will expire immediately, causing the new
	// map to be created.
	return &Cache{
		maxAge: maxAge,
	}
}

// Len returns the total number of cached entries.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.old) + len(c.new)
}

// Evict removes the entry with the given key from the cache if present.
func (c *Cache) Evict(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.new, key)
}

// EvictAll removes all entries from the cache.
func (c *Cache) EvictAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.new = make(map[string]entry)
	c.old = nil
}

// Get returns the value for the given key, using fetch to fetch
// the value if it is not found in the cache.
// If fetch returns an error, the returned error from Get will have
// the same cause.
func (c *Cache) Get(key string, fetch func() (interface{}, error)) (interface{}, error) {
	return c.getAtTime(key, fetch, time.Now())
}

// getAtTime is the internal version of Get, useful for testing; now represents the current
// time.
func (c *Cache) getAtTime(key string, fetch func() (interface{}, error), now time.Time) (interface{}, error) {
	if val, ok := c.cachedValue(key, now); ok {
		return val, nil
	}
	// Fetch the data without the mutex held
	// so that one slow fetch doesn't hold up
	// all the other cache accesses.
	val, err := fetch()
	if err != nil {
		// TODO consider caching cache misses.
		return nil, errgo.Mask(err, errgo.Any)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Add the new cache entry. Because it's quite likely that a
	// large number of cache entries will be initially fetched at
	// the same time, we want to avoid a thundering herd of fetches
	// when they all expire at the same time, so we set the expiry
	// time to a random interval between [now + t.maxAge/2, now +
	// t.maxAge] and so they'll be spread over time without
	// compromising the maxAge value.
	c.new[key] = entry{
		value:  val,
		expire: now.Add(c.maxAge - time.Duration(rand.Int63n(int64(c.maxAge/2)))),
	}
	return val, nil
}

// cachedValue returns any cached value for the given key
// and whether it was found.
func (c *Cache) cachedValue(key string, now time.Time) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now.After(c.expire) {
		c.old = c.new
		c.new = make(map[string]entry)
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
func (c *Cache) entry(m map[string]entry, key string, now time.Time) (entry, bool) {
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
