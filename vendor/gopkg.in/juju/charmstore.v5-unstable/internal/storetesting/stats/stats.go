// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stats // import "gopkg.in/juju/charmstore.v5-unstable/internal/storetesting/stats"

import (
	"time"

	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
)

// CheckCounterSum checks that statistics are properly collected.
// It retries a few times as they are generally collected in background.
func CheckCounterSum(c *gc.C, store *charmstore.Store, key []string, prefix bool, expected int64) {
	var sum int64
	for retry := 0; retry < 10; retry++ {
		time.Sleep(100 * time.Millisecond)
		req := charmstore.CounterRequest{
			Key:    key,
			Prefix: prefix,
		}
		cs, err := store.Counters(&req)
		c.Assert(err, gc.IsNil)
		if sum = cs[0].Count; sum == expected {
			if expected == 0 && retry < 2 {
				continue // Wait a bit to make sure.
			}
			return
		}
	}
	c.Errorf("counter sum for %#v is %d, want %d", key, sum, expected)
}

// CheckSearchTotalDownloads checks that the search index is properly updated.
// It retries a few times as they are generally updated in background.
func CheckSearchTotalDownloads(c *gc.C, store *charmstore.Store, id *charm.URL, expected int64) {
	var doc *charmstore.SearchDoc
	for retry := 0; retry < 10; retry++ {
		var err error
		time.Sleep(100 * time.Millisecond)
		doc, err = store.ES.GetSearchDocument(id)
		c.Assert(err, gc.IsNil)
		if doc.TotalDownloads == expected {
			if expected == 0 && retry < 2 {
				continue // Wait a bit to make sure.
			}
			return
		}
	}
	c.Errorf("total downloads for %#v is %d, want %d", id, doc.TotalDownloads, expected)
}
