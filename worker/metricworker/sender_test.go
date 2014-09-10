// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker_test

import (
	"time"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
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
// It confirms that one metric is sent.
func (s *SenderSuite) TestSender(c *gc.C) {
	notify := make(chan struct{})
	metricworker.PatchNotificationChannel(notify)
	client := &mockClient{}
	worker := metricworker.NewSender(client)
	defer worker.Kill()
	select {
	case <-notify:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("the cleanup function should have fired by now")
	}
	c.Assert(client.calls, gc.DeepEquals, []string{"SendMetrics"})
}

type mockClient struct {
	calls []string
}

func (m *mockClient) CleanupOldMetrics() error {
	m.calls = append(m.calls, "CleanupOldMetrics")
	return nil
}
func (m *mockClient) SendMetrics() error {
	m.calls = append(m.calls, "SendMetrics")
	return nil
}
