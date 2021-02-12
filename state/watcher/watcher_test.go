// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	stdtesting "testing"
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

// Test tuning parameters.
const (
	// worstCase is used for timeouts when timing out
	// will fail the test. Raising this value should
	// not affect the overall running time of the tests
	// unless they fail.
	worstCase = testing.LongWait

	// justLongEnough is used for timeouts that
	// are expected to happen for a test to complete
	// successfully. Reducing this value will make
	// the tests run faster at the expense of making them
	// fail more often on heavily loaded or slow hardware.
	justLongEnough = testing.ShortWait
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type M map[string]interface{}

func assertChange(c *gc.C, watch <-chan watcher.Change, want watcher.Change) {
	select {
	case got := <-watch:
		if got != want {
			c.Fatalf("watch reported %v, want %v", got, want)
		}
	case <-time.After(worstCase):
		c.Fatalf("watch reported nothing, want %v", want)
	}
}

func assertNoChange(c *gc.C, watch <-chan watcher.Change) {
	select {
	case got := <-watch:
		c.Fatalf("watch reported %v, want nothing", got)
	case <-time.After(justLongEnough):
	}
}
