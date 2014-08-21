// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/metricsmanager"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/metricworker"
)

type SenderSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&SenderSuite{})

func (s *SenderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

// TestSend create 2 metrics, one sent and one not sent.
// It confirms that one metric is sent
func (s *SenderSuite) TestSender(c *gc.C) {
	sender := &statetesting.MockSender{}
	s.PatchValue(&state.MetricSend, sender)
	unit := s.Factory.MakeUnit(c, nil)
	now := time.Now()
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &now})
	unsentMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})

	notify := make(chan struct{})
	metricworker.PatchNotificationChannel(notify)
	client := metricsmanager.NewClient(s.APIState)
	worker := metricworker.NewSender(client)
	defer worker.Kill()
	select {
	case <-notify:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("the cleanup function should have fired by now")
	}
	c.Assert(sender.Data, gc.HasLen, 1)
	metric, err := s.State.MetricBatch(unsentMetric.UUID())
	c.Assert(err, gc.IsNil)
	c.Assert(metric.Sent(), jc.IsTrue)

}
