// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"runtime"
	"sync"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/charmstore"
)

// MacaroonCache contains macaroons which are removed at a specified interval.
type MacaroonCache struct {
	// We use a separate struct for the actual cache and a wrapper to export to the
	// user so that the expiry worker which references the internal cache doesn't
	// prevent the exported cache from being garbage collected.
	*cacheInternal
}

// NewMacaroonCache returns a cache containing macaroons which are removed
// after the macaroons' expiry time.
func NewMacaroonCache(clock clock.Clock) *MacaroonCache {
	c := &cacheInternal{clock: clock, macaroons: make(map[string]*macaroonEntry)}
	cache := &MacaroonCache{c}
	// The interval to run the expiry worker is somewhat arbitrary.
	// Expired macaroons will be re-issued as needed; we just want to ensure
	// that those which fall out of use are eventually cleaned up.
	c.runExpiryWorker(10 * time.Minute)
	runtime.SetFinalizer(cache, stopMacaroonCacheExpiryWorker)
	return cache
}

type expiryWorker struct {
	clock    clock.Clock
	interval time.Duration
	stop     chan bool
}

func (w *expiryWorker) loop(c *cacheInternal) {
	for {
		select {
		case <-w.clock.After(w.interval):
			c.deleteExpired()
		case <-w.stop:
			return
		}
	}
}

type macaroonEntry struct {
	ms         macaroon.Slice
	expiryTime *time.Time
}

func (i *macaroonEntry) expired(clock clock.Clock) bool {
	return i.expiryTime != nil && i.expiryTime.Before(clock.Now())
}

type cacheInternal struct {
	sync.Mutex
	clock clock.Clock

	macaroons    map[string]*macaroonEntry
	expiryWorker *expiryWorker
}

// Upsert inserts or updates a macaroon slice in the cache.
func (c *cacheInternal) Upsert(token string, ms macaroon.Slice) {
	c.Lock()
	defer c.Unlock()

	var et *time.Time
	if expiryTime, ok := checkers.MacaroonsExpiryTime(charmstore.MacaroonNamespace, ms); ok {
		et = &expiryTime
	}
	c.macaroons[token] = &macaroonEntry{
		ms:         ms,
		expiryTime: et,
	}
}

// Get returns a macaroon slice from the cache, and a bool indicating
// if the slice for the key was found.
func (c *cacheInternal) Get(k string) (macaroon.Slice, bool) {
	c.Lock()
	defer c.Unlock()

	entry, found := c.macaroons[k]
	if !found {
		return nil, false
	}
	if entry.expired(c.clock) {
		delete(c.macaroons, k)
		return nil, false
	}
	return entry.ms, true
}

func (c *cacheInternal) deleteExpired() {
	c.Lock()
	defer c.Unlock()

	for k, v := range c.macaroons {
		if v.expired(c.clock) {
			delete(c.macaroons, k)
		}
	}
}

func stopMacaroonCacheExpiryWorker(mc *MacaroonCache) {
	mc.expiryWorker.stop <- true
}

func (c *cacheInternal) runExpiryWorker(interval time.Duration) {
	w := &expiryWorker{
		interval: interval,
		clock:    c.clock,
		stop:     make(chan bool),
	}
	c.expiryWorker = w
	go w.loop(c)
}
