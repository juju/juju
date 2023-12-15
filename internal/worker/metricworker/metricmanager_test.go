// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker_test

import (
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/metricworker"
	coretesting "github.com/juju/juju/testing"
)

type MetricManagerSuite struct{}

var _ = gc.Suite(&MetricManagerSuite{})

func (s *MetricManagerSuite) TestRunner(c *gc.C) {
	notify := make(chan string, 2)
	var client mockClient
	_, err := metricworker.NewMetricsManager(&client, notify, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	expectedCalls := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case call := <-notify:
			expectedCalls[call] = true
		case <-time.After(coretesting.LongWait):
			c.Logf("we should have received a notification by now")
		}
	}

	c.Check(expectedCalls["senderCalled"], jc.IsTrue)
	c.Check(expectedCalls["cleanupCalled"], jc.IsTrue)
}
