// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package loki

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus"
)

type clientSuite struct{}

func TestClientSuite(t *testing.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) TestNewClientEmptyEndpoint(c *tc.C) {
	_, err := NewClient("", DefaultConfig())
	c.Assert(err, tc.ErrorMatches, "endpoint must not be empty")
}

func (s *clientSuite) TestNewClientDefaults(c *tc.C) {
	cfg := DefaultConfig()
	client, err := NewClient("http://loki:3100", cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)
	c.Check(client.cfg.BatchSize, tc.Equals, 100)
	c.Check(
		client.cfg.FlushInterval,
		tc.Equals,
		10*time.Second,
	)
	c.Check(client.cfg.MaxRetries, tc.Equals, 3)
	c.Check(
		client.cfg.InitialBackoff,
		tc.Equals,
		500*time.Millisecond,
	)
	c.Check(
		client.cfg.MaxBackoff,
		tc.Equals,
		30*time.Second,
	)
}

func (s *clientSuite) TestNewClientUsesDefaultHTTPClient(c *tc.C) {
	srv, payloads := newTestServer(c)
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BatchSize = 1
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	err = client.Push(Record{
		Timestamp:      time.Now(),
		Line:           "default http client",
		ControllerUUID: "controller",
		ModelUUID:      "model",
		AgentID:        "machine-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 1)
	c.Check(p.Streams[0].Values[0].Line, tc.Equals, "default http client")
}

func (s *clientSuite) TestNewClientZeroRetriesIsValid(c *tc.C) {
	cfg := testConfig()
	cfg.MaxRetries = 0
	client, err := NewClient("http://loki:3100", cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)
	c.Check(client.cfg.MaxRetries, tc.Equals, 0)
}

func (s *clientSuite) TestConfigValidateRejectsEmptyConfig(c *tc.C) {
	err := Config{}.Validate()
	c.Assert(err, tc.ErrorMatches, "BatchSize must be positive")
}

func (s *clientSuite) TestConfigValidateRejectsNegativeMaxRetries(c *tc.C) {
	cfg := testConfig()
	cfg.MaxRetries = -1
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "MaxRetries must not be negative")
}

func (s *clientSuite) TestConfigValidateRejectsNilClock(c *tc.C) {
	cfg := testConfig()
	cfg.Clock = nil
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "Clock must not be nil")
}

func (s *clientSuite) TestPushNoRetriesWhenMaxRetriesZero(c *tc.C) {
	var attempts atomic.Int32
	errCh := make(chan error, 1)

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 1
	cfg.MaxRetries = 0
	cfg.OnError = func(err error) {
		errCh <- err
	}

	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	err = client.Push(Record{
		Timestamp:      time.Now(),
		Line:           "no retries",
		ControllerUUID: "controller",
		ModelUUID:      "model",
		AgentID:        "machine-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	pushErr := waitError(c, errCh)
	c.Check(pushErr, tc.ErrorMatches, ".*loki returned status 500")
	c.Check(attempts.Load(), tc.Equals, int32(1))
}

func (s *clientSuite) TestPushUsesWallClock(c *tc.C) {
	srv, payloads := newTestServer(c)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 1

	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	err = client.Push(Record{
		Timestamp:      time.Now(),
		Line:           "clock default",
		ControllerUUID: "controller",
		ModelUUID:      "model",
		AgentID:        "machine-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 1)
	c.Check(p.Streams[0].Values[0].Line, tc.Equals, "clock default")
}

func (s *clientSuite) TestPushFlushOnBatchSize(c *tc.C) {
	srv, payloads := newTestServer(c)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 3
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	ts := time.Now()
	for range 3 {
		err := client.Push(Record{
			Timestamp:      ts,
			Line:           "line",
			ControllerUUID: "controller",
			ModelUUID:      "model",
			AgentID:        "machine-0",
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 1)
	c.Check(p.Streams[0].Values, tc.HasLen, 3)
}

func (s *clientSuite) TestPushSyncFlush(c *tc.C) {
	srv, payloads := newTestServer(c)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 2
	asyncFlush := false
	cfg.AsyncFlush = &asyncFlush
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	ts := time.Now()
	err = client.Push(
		Record{Timestamp: ts, Line: "s1", ControllerUUID: "controller",
			ModelUUID: "model", AgentID: "machine-0"},
		Record{Timestamp: ts, Line: "s2", ControllerUUID: "controller",
			ModelUUID: "model", AgentID: "machine-0"},
	)
	c.Assert(err, tc.ErrorIsNil)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 1)
	c.Check(p.Streams[0].Values, tc.HasLen, 2)
}

func (s *clientSuite) TestPushFlushOnInterval(c *tc.C) {
	srv, payloads := newTestServer(c)
	defer srv.Close()

	cfg := testConfig()
	cfg.FlushInterval = 50 * time.Millisecond
	cfg.BatchSize = 1000
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	err = client.Push(Record{
		Timestamp:      time.Now(),
		Line:           "timer flush",
		ControllerUUID: "controller",
		ModelUUID:      "model",
		AgentID:        "machine-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 1)
	c.Check(p.Streams[0].Values[0].Line, tc.Equals, "timer flush")
}

func (s *clientSuite) TestPushGroupsByTopologyLabels(c *tc.C) {
	srv, payloads := newTestServer(c)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 3
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	ts := time.Now()
	err = client.Push(
		Record{
			Timestamp:      ts,
			Line:           "a1",
			ControllerUUID: "controller",
			ModelUUID:      "model",
			AgentID:        "machine-0",
		},
		Record{
			Timestamp:      ts,
			Line:           "b1",
			ControllerUUID: "controller",
			ModelUUID:      "model",
			AgentID:        "unit-app-0",
		},
		Record{
			Timestamp:      ts,
			Line:           "a2",
			ControllerUUID: "controller",
			ModelUUID:      "model",
			AgentID:        "machine-0",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 2)
	c.Check(p.Streams[0].Stream["juju_agent"], tc.Equals, "machine-0")
	c.Check(p.Streams[0].Values, tc.HasLen, 2)
	c.Check(p.Streams[1].Stream["juju_agent"], tc.Equals, "unit-app-0")
	c.Check(p.Streams[1].Values, tc.HasLen, 1)
}

func (s *clientSuite) TestPushBatching(c *tc.C) {
	var requestCount atomic.Int32
	done := make(chan struct{}, 10)
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusNoContent)
			done <- struct{}{}
		}),
	)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 3
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	ts := time.Now()
	// Send 6 records: triggers 2 flushes of 3.
	for range 6 {
		err := client.Push(Record{
			Timestamp:      ts,
			Line:           "line",
			ControllerUUID: "controller",
			ModelUUID:      "model",
			AgentID:        "machine-0",
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	// Wait for both HTTP requests to arrive.
	for range 2 {
		select {
		case <-done:
		case <-c.Context().Done():
			c.Fatal("timed out waiting for request")
		}
	}
	c.Check(requestCount.Load(), tc.Equals, int32(2))
}

func (s *clientSuite) TestPushRetryOnServerError(c *tc.C) {
	var attempts atomic.Int32
	done := make(chan struct{}, 1)
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempts.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			done <- struct{}{}
		}),
	)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 1
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	err = client.Push(Record{
		Timestamp:      time.Now(),
		Line:           "retry me",
		ControllerUUID: "controller",
		ModelUUID:      "model",
		AgentID:        "machine-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for successful push")
	}
	c.Check(attempts.Load(), tc.Equals, int32(2))
}

func (s *clientSuite) TestPushRetryOnTooManyRequests(c *tc.C) {
	var attempts atomic.Int32
	done := make(chan struct{}, 1)
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempts.Add(1)
			if n <= 2 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
			done <- struct{}{}
		}),
	)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 1
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	err = client.Push(Record{
		Timestamp:      time.Now(),
		Line:           "throttled",
		ControllerUUID: "controller",
		ModelUUID:      "model",
		AgentID:        "machine-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for successful push")
	}
	c.Check(attempts.Load(), tc.Equals, int32(3))
}

func (s *clientSuite) TestOnErrorCalledOnPushFailure(c *tc.C) {
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)
	defer srv.Close()

	errCh := make(chan error, 1)
	cfg := testConfig()
	cfg.BatchSize = 1
	cfg.MaxRetries = 1
	cfg.OnError = func(err error) {
		errCh <- err
	}
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	err = client.Push(Record{
		Timestamp:      time.Now(),
		Line:           "will fail",
		ControllerUUID: "controller",
		ModelUUID:      "model",
		AgentID:        "machine-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case pushErr := <-errCh:
		c.Check(
			pushErr, tc.ErrorMatches,
			".*loki returned status 500",
		)
	case <-c.Context().Done():
		c.Fatal("timed out waiting for OnError callback")
	}
}

func (s *clientSuite) TestPushFailureWithNilOnErrorCallback(c *tc.C) {
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 1
	cfg.MaxRetries = 0
	cfg.OnError = nil

	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	err = client.Push(Record{
		Timestamp:      time.Now(),
		Line:           "will fail",
		ControllerUUID: "controller",
		ModelUUID:      "model",
		AgentID:        "machine-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	waitForPushErrors(c, client, 1)
}

func (s *clientSuite) TestPushDropsOldestWhenQueueFull(c *tc.C) {
	block := make(chan struct{})
	started := make(chan struct{}, 1)
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-block
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	defer srv.Close()

	drops := make(chan int, 3)
	cfg := testConfig()
	cfg.BatchSize = 1
	cfg.BufferSize = 2
	cfg.MaxRetries = 0
	asyncFlush := false
	cfg.AsyncFlush = &asyncFlush
	cfg.OnDrop = func(count int) {
		drops <- count
	}

	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)

	ts := time.Now()
	err = client.Push(Record{Timestamp: ts, Line: "first"})
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-started:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for first request")
	}

	// The first record blocks the worker in HTTP I/O. The queue can then fill
	// and must drop oldest records without blocking this caller.
	err = client.Push(
		Record{Timestamp: ts, Line: "second"},
		Record{Timestamp: ts, Line: "third"},
		Record{Timestamp: ts, Line: "fourth"},
	)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case count := <-drops:
		c.Check(count, tc.Equals, 1)
	case <-c.Context().Done():
		c.Fatal("timed out waiting for drop callback")
	}
	c.Check(client.Report(c.Context())["dropped"], tc.Equals, uint64(1))

	close(block)
	killAndWait(c, client)
}

func (s *clientSuite) TestPushDropsOldestWhenQueueFullWithoutOnDropCallback(c *tc.C) {
	block := make(chan struct{})
	started := make(chan struct{}, 1)
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-block
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 1
	cfg.BufferSize = 2
	cfg.MaxRetries = 0
	cfg.OnDrop = nil
	asyncFlush := false
	cfg.AsyncFlush = &asyncFlush

	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)

	ts := time.Now()
	err = client.Push(Record{Timestamp: ts, Line: "first"})
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-started:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for first request")
	}

	err = client.Push(
		Record{Timestamp: ts, Line: "second"},
		Record{Timestamp: ts, Line: "third"},
		Record{Timestamp: ts, Line: "fourth"},
	)
	c.Assert(err, tc.ErrorIsNil)

	waitForDropped(c, client, 1)

	close(block)
	killAndWait(c, client)
}

func (s *clientSuite) TestKillDrainsBufferedRecords(c *tc.C) {
	srv, payloads := newTestServer(c)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 1000
	// Long interval so only kill-drain triggers flush.
	cfg.FlushInterval = time.Minute
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)

	ts := time.Now()
	err = client.Push(
		Record{
			Timestamp:      ts,
			Line:           "drain me",
			ControllerUUID: "controller",
			ModelUUID:      "model",
			AgentID:        "machine-0",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	killAndWait(c, client)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 1)
	c.Check(
		p.Streams[0].Values[0].Line,
		tc.Equals,
		"drain me",
	)
}

func (s *clientSuite) TestPushNoTopologyLabels(c *tc.C) {
	srv, payloads := newTestServer(c)
	defer srv.Close()

	cfg := testConfig()
	cfg.BatchSize = 1
	client, err := NewClient(srv.URL, cfg)
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	err = client.Push(Record{
		Timestamp: time.Now(),
		Line:      "no labels",
	})
	c.Assert(err, tc.ErrorIsNil)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 1)
	c.Check(p.Streams[0].Values, tc.HasLen, 1)
}

func (s *clientSuite) TestMetricsCollector(c *tc.C) {
	client, err := NewClient("http://loki:3100", testConfig())
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)

	atomic.StoreUint64(&client.stats.Sent, 3)
	atomic.StoreUint64(&client.stats.Dropped, 2)
	atomic.StoreUint64(&client.stats.PushErrors, 1)

	reg := prometheus.NewRegistry()
	c.Assert(reg.Register(NewMetricsCollector(client)), tc.ErrorIsNil)

	metrics, err := reg.Gather()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(metrics, tc.HasLen, 3)

	got := make(map[string]float64)
	for _, family := range metrics {
		c.Assert(family.Metric, tc.HasLen, 1)
		got[family.GetName()] = family.Metric[0].GetCounter().GetValue()
	}

	c.Check(got["juju_loki_forwarder_sent_total"], tc.Equals, 3.0)
	c.Check(got["juju_loki_forwarder_dropped_total"], tc.Equals, 2.0)
	c.Check(got["juju_loki_forwarder_push_errors_total"], tc.Equals, 1.0)
}

func (s *clientSuite) TestBuildPayload(c *tc.C) {
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	records := []Record{
		{
			Timestamp:      ts,
			Line:           "a1",
			ControllerUUID: "controller",
			ModelUUID:      "model",
			AgentID:        "machine-0",
			Fields: map[string]string{
				"module": "apiserver",
			},
			TraceID: "0123456789abcdef0123456789abcdef",
			SpanID:  "0123456789abcdef",
		},
		{
			Timestamp:      ts.Add(time.Second),
			Line:           "b1",
			ControllerUUID: "controller",
			ModelUUID:      "model",
			AgentID:        "unit-app-0",
		},
		{
			Timestamp:      ts.Add(2 * time.Second),
			Line:           "a2",
			ControllerUUID: "controller",
			ModelUUID:      "model",
			AgentID:        "machine-0",
			Fields: map[string]string{
				"request_id": "req-123",
			},
			TraceID: "NOT-CANONICAL",
			SpanID:  "0123456789abcdef",
		},
	}

	payload := buildPayload(records, DefaultServiceName)
	c.Assert(payload.Streams, tc.HasLen, 2)

	c.Check(
		payload.Streams[0].Stream,
		tc.DeepEquals,
		map[string]string{
			"service_name":    "juju",
			"juju_controller": "controller",
			"juju_model":      "model",
			"juju_agent":      "machine-0",
		},
	)
	c.Check(payload.Streams[0].Values, tc.HasLen, 2)
	c.Check(payload.Streams[0].Values[0].Line, tc.Equals, "a1")
	c.Check(payload.Streams[0].Values[0].Fields, tc.DeepEquals, map[string]string{
		"module":   "apiserver",
		"trace_id": "0123456789abcdef0123456789abcdef",
		"span_id":  "0123456789abcdef",
	})
	c.Check(payload.Streams[0].Values[1].Line, tc.Equals, "a2")
	c.Check(payload.Streams[0].Values[1].Fields, tc.DeepEquals, map[string]string{
		"request_id": "req-123",
		"span_id":    "0123456789abcdef",
	})

	c.Check(
		payload.Streams[1].Stream,
		tc.DeepEquals,
		map[string]string{
			"service_name":    "juju",
			"juju_controller": "controller",
			"juju_model":      "model",
			"juju_agent":      "unit-app-0",
		},
	)
	c.Check(payload.Streams[1].Values, tc.HasLen, 1)
}

func (s *clientSuite) TestLabelKey(c *tc.C) {
	labels := map[string]string{
		"z": "3", "a": "1", "m": "2",
	}
	key := labelKey(labels)
	c.Check(key, tc.Equals, "a=1,m=2,z=3")
}

func (s *clientSuite) TestLabelKeyEmpty(c *tc.C) {
	c.Check(labelKey(nil), tc.Equals, "")
	c.Check(labelKey(map[string]string{}), tc.Equals, "")
}

// testConfig returns a Config suitable for tests with fast
// flushing and minimal retry delays.
func testConfig() Config {
	return Config{
		BatchSize:      100,
		BufferSize:     500,
		FlushInterval:  50 * time.Millisecond,
		MaxRetries:     3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Clock:          clock.WallClock,
		OnError:        func(error) {},
		OnDrop:         func(int) {},
		HTTPClient:     http.DefaultClient,
	}
}

// newTestServer creates an httptest server that records push
// payloads on a buffered channel.
func newTestServer(
	c *tc.C,
) (*httptest.Server, <-chan pushPayload) {
	payloads := make(chan pushPayload, 100)
	srv := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter, r *http.Request,
		) {
			var p pushPayload
			err := json.NewDecoder(r.Body).Decode(&p)
			c.Assert(err, tc.ErrorIsNil)
			payloads <- p
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	return srv, payloads
}

// waitPayload waits for a payload on the channel, failing the
// test if none arrives within 5 seconds.
func waitPayload(
	c *tc.C, ch <-chan pushPayload,
) pushPayload {
	select {
	case p := <-ch:
		return p
	case <-c.Context().Done():
		c.Fatal("timed out waiting for payload")
		return pushPayload{}
	}
}

func waitError(c *tc.C, ch <-chan error) error {
	select {
	case err := <-ch:
		return err
	case <-c.Context().Done():
		c.Fatal("timed out waiting for error callback")
		return nil
	}
}

func killAndWait(c *tc.C, client *Client) {
	client.Kill()
	err := client.Wait()
	c.Assert(err, tc.ErrorIsNil)
}

func waitForDropped(c *tc.C, client *Client, expected uint64) {
	for {
		if client.Report(c.Context())["dropped"] == expected {
			return
		}
		select {
		case <-c.Context().Done():
			c.Fatalf("timed out waiting for dropped count %d", expected)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func waitForPushErrors(c *tc.C, client *Client, expected uint64) {
	for {
		if client.Report(c.Context())["push-errors"] == expected {
			return
		}
		select {
		case <-c.Context().Done():
			c.Fatalf("timed out waiting for push-errors count %d", expected)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
