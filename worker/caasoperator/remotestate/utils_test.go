// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

func assertNoNotifyEvent(c *gc.C, ch <-chan struct{}, event string) {
	select {
	case <-ch:
		c.Fatalf("unexpected %s", event)
	case <-time.After(testing.ShortWait):
	}
}

func assertNotifyEvent(c *gc.C, ch <-chan struct{}, activity string) {
	select {
	case <-ch:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out %s", activity)
		panic("unreachable")
	}
}
