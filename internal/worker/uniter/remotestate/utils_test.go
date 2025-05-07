// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

func assertNoNotifyEvent(c *tc.C, ch <-chan struct{}, event string) {
	select {
	case <-ch:
		c.Fatalf("unexpected %s", event)
	case <-time.After(testing.ShortWait):
	}
}

func assertNotifyEvent(c *tc.C, ch <-chan struct{}, activity string) {
	select {
	case <-ch:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out %s", activity)
		panic("unreachable")
	}
}
