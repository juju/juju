// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"sync"
	"time"

	"github.com/juju/clock"
)

const (
	// defaultNamespaceExpiry holds the duration for how long a namespace
	// should be cached for.
	defaultNamespaceExpiry = time.Minute
)

// nsCache holds a cache of namespaces that have been validated to exist.
type nsCache struct {
	mutex      sync.RWMutex
	namespaces map[string]time.Time
	clock      clock.Clock
	expiry     time.Duration
}

func newNSCache(expiry time.Duration, clock clock.Clock) *nsCache {
	return &nsCache{
		namespaces: make(map[string]time.Time),
		clock:      clock,
		expiry:     expiry,
	}
}

// Set adds the given namespace to the cache.
func (c *nsCache) Set(namespace string, known bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// If we know the namespace exists, either insert it or update the existing
	// entry.
	if known {
		c.namespaces[namespace] = c.clock.Now().Add(c.expiry)
		return
	}

	// Otherwise just delete it from the namespaces map, even if it doesn't
	// exist.
	delete(c.namespaces, namespace)
}

// Exists returns whether the given namespace exists in the cache.
func (c *nsCache) Exists(namespace string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	expiry, ok := c.namespaces[namespace]
	if !ok {
		return false
	}
	return c.clock.Now().Before(expiry)
}

func (c *nsCache) Remove(namespace string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.namespaces, namespace)
}

// Flush removes all namespaces from the cache that have expired. This just
// ensures that we keep a nice and clean cache.
func (c *nsCache) Flush() {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	now := c.clock.Now()
	for namespace, expiry := range c.namespaces {
		if now.After(expiry) {
			delete(c.namespaces, namespace)
		}
	}
}
