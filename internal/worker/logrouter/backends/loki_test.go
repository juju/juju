// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/loggo/v3"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/loki"
	"github.com/juju/juju/internal/worker/logsender"
)

type lokiSuite struct{}

func TestLokiSuite(t *testing.T) {
	tc.Run(t, &lokiSuite{})
}

func (s *lokiSuite) TestUsesClientInterfaceAndManagesLifecycle(c *tc.C) {
	client := newRecordingLokiClient()
	httpClient := &http.Client{}

	w, err := NewLoki(LokiConfig{
		BackendBufferSize: 1,
		ClientConfig: loki.Config{
			HTTPClient: httpClient,
		},
		Endpoint:       "http://loki/loki/api/v1/push",
		ControllerUUID: "controller",
		ModelUUID:      "model",
		AgentID:        "machine-0",
		NewClient: func(endpoint string, cfg loki.Config) (LokiClient, error) {
			c.Check(endpoint, tc.Equals, "http://loki/loki/api/v1/push")
			c.Check(cfg.HTTPClient, tc.Equals, httpClient)
			return client, nil
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	w.LogRecords() <- &logsender.LogRecord{
		Time:     time.Now(),
		Module:   "test.module",
		Location: "worker.go:10",
		Level:    loggo.INFO,
		Message:  "hello loki",
	}

	got := client.waitRecord(c)
	c.Check(got.Line, tc.Equals, "hello loki")
	c.Check(got.ControllerUUID, tc.Equals, "controller")
	c.Check(got.ModelUUID, tc.Equals, "model")
	c.Check(got.AgentID, tc.Equals, "machine-0")
	c.Check(got.Fields, tc.DeepEquals, map[string]string{
		"module":   "test.module",
		"location": "worker.go:10",
		"level":    "INFO",
	})

	workertest.CleanKill(c, w)

	c.Check(client.killed.Load(), tc.IsTrue)
	c.Check(client.waited.Load(), tc.IsTrue)
}

type recordingLokiClient struct {
	tomb    tomb.Tomb
	records chan loki.Record
	killed  atomic.Bool
	waited  atomic.Bool
}

func newRecordingLokiClient() *recordingLokiClient {
	c := &recordingLokiClient{
		records: make(chan loki.Record, 1),
	}
	c.tomb.Go(func() error {
		<-c.tomb.Dying()
		return nil
	})
	return c
}

func (c *recordingLokiClient) Push(records ...loki.Record) error {
	for _, record := range records {
		c.records <- record
	}
	return nil
}

func (c *recordingLokiClient) Kill() {
	c.killed.Store(true)
	c.tomb.Kill(nil)
}

func (c *recordingLokiClient) Wait() error {
	c.waited.Store(true)
	return c.tomb.Wait()
}

func (c *recordingLokiClient) waitRecord(t *tc.C) loki.Record {
	select {
	case record := <-c.records:
		return record
	case <-t.Context().Done():
		t.Fatalf("timed out waiting for loki record")
	}
	return loki.Record{}
}

func (c *recordingLokiClient) Report(ctx context.Context) map[string]any {
	return map[string]any{
		"records": len(c.records),
	}
}
