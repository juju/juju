// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker_test

import (
	"time"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/metricworker"
)

type CleanupSuite struct {
}

var _ = gc.Suite(&CleanupSuite{})

// TestCleaner create 2 metrics, one old and one new.
// After a single run of the cleanup worker it expects the
// old one to be deleted
func (s *CleanupSuite) TestCleaner(c *gc.C) {
	notify := make(chan string)
	cleanup := metricworker.PatchNotificationChannel(notify)
	defer cleanup()
	client := &mockClient{}
	worker := metricworker.NewCleanup(client)
	defer worker.Kill()
	select {
	case <-notify:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("the cleanup function should have fired by now")
	}
	c.Assert(client.calls, gc.DeepEquals, []string{"CleanupOldMetrics"})
}
