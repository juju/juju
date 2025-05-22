// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/canonical/pebble/client"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter"
	"github.com/juju/juju/internal/worker/uniter/container"
)

type pebbleNoticerSuite struct {
	clock             *testclock.Clock
	worker            worker.Worker
	clients           map[string]*fakePebbleClient
	workloadEventChan chan string
	workloadEvents    container.WorkloadEvents
}

func TestPebbleNoticerSuite(t *stdtesting.T) {
	tc.Run(t, &pebbleNoticerSuite{})
}

func (s *pebbleNoticerSuite) setUpWorker(c *tc.C, containerNames []string) {
	s.clock = testclock.NewClock(time.Time{})
	s.workloadEventChan = make(chan string)
	s.workloadEvents = container.NewWorkloadEvents()
	s.clients = make(map[string]*fakePebbleClient)
	for _, name := range containerNames {
		s.clients[name] = &fakePebbleClient{
			clock:       testclock.NewClock(time.Time{}),
			noticeAdded: make(chan *client.Notice, 1), // buffered so AddNotice doesn't block
		}
	}
	newClient := func(cfg *client.Config) (uniter.PebbleClient, error) {
		c.Assert(cfg.Socket, tc.Matches, pebbleSocketPathRegexpString)
		matches := pebbleSocketPathRegexp.FindAllStringSubmatch(cfg.Socket, 1)
		return s.clients[matches[0][1]], nil
	}
	s.worker = uniter.NewPebbleNoticer(loggertesting.WrapCheckLog(c), s.clock, containerNames, s.workloadEventChan, s.workloadEvents, newClient)
}

func (s *pebbleNoticerSuite) waitWorkloadEvent(c *tc.C, expected container.WorkloadEvent) {
	select {
	case id := <-s.workloadEventChan:
		event, callback, err := s.workloadEvents.GetWorkloadEvent(id)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(event, tc.DeepEquals, expected)
		callback(nil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for event")
	}
}

func (s *pebbleNoticerSuite) TestWaitNotices(c *tc.C) {
	s.setUpWorker(c, []string{"c1"})
	defer workertest.CleanKill(c, s.worker)

	// Simulate a WaitNotices timeout.
	s.clients["c1"].clock.WaitAdvance(30*time.Second, testing.ShortWait, 1)

	// The first notice will always be handled.
	lastRepeated := time.Now()
	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:           "1",
		Type:         "custom",
		Key:          "a.b/c",
		LastRepeated: lastRepeated,
	})
	s.waitWorkloadEvent(c, container.WorkloadEvent{
		Type:         container.CustomNoticeEvent,
		WorkloadName: "c1",
		NoticeID:     "1",
		NoticeType:   "custom",
		NoticeKey:    "a.b/c",
	})

	// Another notice with an earlier LastRepeated time will be skipped.
	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:           "2",
		Type:         "custom",
		Key:          "a.b/d",
		LastRepeated: lastRepeated.Add(-time.Second),
	})
	select {
	case <-s.workloadEventChan:
		c.Fatalf("shouldn't see this notice")
	case <-time.After(testing.ShortWait):
	}

	// A new notice with a later LastRepeated time will be handled.
	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:           "3",
		Type:         "custom",
		Key:          "a.b/e",
		LastRepeated: lastRepeated.Add(time.Second),
	})
	s.waitWorkloadEvent(c, container.WorkloadEvent{
		Type:         container.CustomNoticeEvent,
		WorkloadName: "c1",
		NoticeID:     "3",
		NoticeType:   "custom",
		NoticeKey:    "a.b/e",
	})
}

// TestCheckFailed verifies that a change-updated notice that is of kind
// perform-check and has a status of Error results in a CheckFailed event.
func (s *pebbleNoticerSuite) TestCheckFailed(c *tc.C) {
	s.setUpWorker(c, []string{"c1"})
	defer workertest.CleanKill(c, s.worker)

	s.clients["c1"].AddChange(c, &client.Change{
		ID:     "42",
		Kind:   "perform-check",
		Status: "Error",
	})
	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:           "1",
		Type:         "change-update",
		Key:          "42",
		LastRepeated: time.Now(),
		LastData:     map[string]string{"kind": "perform-check", "check-name": "http-check"},
	})
	s.waitWorkloadEvent(c, container.WorkloadEvent{
		Type:         container.CheckFailedEvent,
		WorkloadName: "c1",
		CheckName:    "http-check",
	})
}

// TestCheckRecovered verifies that a change-updated notice that is of kind
// recover-check and has a status of Done results in a CheckRecovered event.
func (s *pebbleNoticerSuite) TestCheckRecovered(c *tc.C) {
	s.setUpWorker(c, []string{"c1"})
	defer workertest.CleanKill(c, s.worker)

	s.clients["c1"].AddChange(c, &client.Change{
		ID:     "42",
		Kind:   "recover-check",
		Status: "Done",
	})
	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:           "1",
		Type:         "change-update",
		Key:          "42",
		LastRepeated: time.Now(),
		LastData:     map[string]string{"kind": "recover-check", "check-name": "tcp-check"},
	})
	s.waitWorkloadEvent(c, container.WorkloadEvent{
		Type:         container.CheckRecoveredEvent,
		WorkloadName: "c1",
		CheckName:    "tcp-check",
	})
}

func (s *pebbleNoticerSuite) TestWaitNoticesError(c *tc.C) {
	s.setUpWorker(c, []string{"c1"})
	defer workertest.CleanKill(c, s.worker)

	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:   "1",
		Type: "error",
		Key:  "WaitNotices error!",
	})
	s.clock.WaitAdvance(testing.LongWait, time.Second, 1)

	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:   "2",
		Type: "custom",
		Key:  "a.b/c",
	})
	s.waitWorkloadEvent(c, container.WorkloadEvent{
		Type:         container.CustomNoticeEvent,
		WorkloadName: "c1",
		NoticeID:     "2",
		NoticeType:   "custom",
		NoticeKey:    "a.b/c",
	})
}

func (s *pebbleNoticerSuite) TestIgnoreUnhandledType(c *tc.C) {
	s.setUpWorker(c, []string{"c1"})
	defer workertest.CleanKill(c, s.worker)

	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:   "1",
		Type: "unhandled",
		Key:  "some-key",
	})

	select {
	case <-s.workloadEventChan:
		c.Fatalf("should ignore this notice")
	case <-time.After(testing.ShortWait):
	}
}

func (s *pebbleNoticerSuite) TestFailedChangeNotFound(c *tc.C) {
	s.setUpWorker(c, []string{"c1"})
	defer workertest.CleanKill(c, s.worker)

	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:           "1",
		Type:         "change-update",
		Key:          "42",
		LastRepeated: time.Now(),
		LastData:     map[string]string{"kind": "perform-check", "check-name": "http-check"},
	})
	s.waitWorkloadEvent(c, container.WorkloadEvent{
		Type:         container.CheckFailedEvent,
		WorkloadName: "c1",
		CheckName:    "http-check",
	})
}

func (s *pebbleNoticerSuite) TestRecoveredChangeNotFound(c *tc.C) {
	s.setUpWorker(c, []string{"c1"})
	defer workertest.CleanKill(c, s.worker)

	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:           "1",
		Type:         "change-update",
		Key:          "42",
		LastRepeated: time.Now(),
		LastData:     map[string]string{"kind": "recover-check", "check-name": "http-check"},
	})
	s.waitWorkloadEvent(c, container.WorkloadEvent{
		Type:         container.CheckRecoveredEvent,
		WorkloadName: "c1",
		CheckName:    "http-check",
	})
}

func (s *pebbleNoticerSuite) TestOtherChangeError(c *tc.C) {
	s.setUpWorker(c, []string{"c1"})
	defer func() {
		err := workertest.CheckKilled(c, s.worker)
		c.Assert(err, tc.ErrorMatches, ".*some other error")
	}()

	s.clients["c1"].changeErr = fmt.Errorf("some other error")
	s.clients["c1"].AddNotice(c, &client.Notice{
		ID:           "1",
		Type:         "change-update",
		Key:          "42",
		LastRepeated: time.Now(),
		LastData:     map[string]string{"kind": "perform-check", "check-name": "http-check"},
	})

	select {
	case <-s.workloadEventChan:
		c.Fatalf("should ignore this notice")
	case <-time.After(testing.ShortWait):
	}
}

func (s *pebbleNoticerSuite) TestMultipleContainers(c *tc.C) {
	s.setUpWorker(c, []string{"c1", "c2"})
	defer workertest.CleanKill(c, s.worker)

	for i := 1; i <= 2; i++ {
		name := fmt.Sprintf("c%d", i)
		s.clients[name].AddNotice(c, &client.Notice{
			ID:   "1",
			Type: "custom",
			Key:  "example.com/" + name,
		})
		s.waitWorkloadEvent(c, container.WorkloadEvent{
			Type:         container.CustomNoticeEvent,
			WorkloadName: name,
			NoticeID:     "1",
			NoticeType:   "custom",
			NoticeKey:    "example.com/" + name,
		})
	}
}
