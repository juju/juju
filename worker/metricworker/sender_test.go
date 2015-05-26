// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker_test

import (
	"sync"
	"time"

	gc "gopkg.in/check.v1"

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
	notify := make(chan string)
	cleanup := metricworker.PatchNotificationChannel(notify)
	defer cleanup()
	client := &mockClient{}
	worker := metricworker.NewSender(client)
	select {
	case <-notify:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("the cleanup function should have fired by now")
	}
	c.Assert(client.calls, gc.DeepEquals, []string{"SendMetrics"})
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

type mockClient struct {
	calls []string
	lock  sync.RWMutex
}

func (m *mockClient) CleanupOldMetrics() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.calls = append(m.calls, "CleanupOldMetrics")
	return nil
}
func (m *mockClient) SendMetrics() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.calls = append(m.calls, "SendMetrics")
	return nil
}
