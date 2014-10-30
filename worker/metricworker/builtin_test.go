// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker_test

import (
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/metricworker"
)

type BuiltinSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&BuiltinSuite{})

func (s *BuiltinSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

// TestSend create 2 metrics, one sent and one not sent.
// It confirms that one metric is sent.
func (s *BuiltinSuite) TestBuiltin(c *gc.C) {
	notify := make(chan struct{})
	cleanup := metricworker.PatchNotificationChannel(notify)
	defer cleanup()
	client := &mockClient{}
	worker := metricworker.NewBuiltin(client)
	select {
	case <-notify:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("the builtin worker function should have fired by now")
	}
	c.Assert(client.calls, gc.DeepEquals, []string{"AddBuiltinMetrics"})
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}
