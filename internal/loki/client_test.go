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

func (s *clientSuite) TestNewClientZeroRetriesIsValid(c *tc.C) {
	client, err := NewClient("http://loki:3100", Config{})
	c.Assert(err, tc.ErrorIsNil)
	defer killAndWait(c, client)
	c.Check(client.cfg.MaxRetries, tc.Equals, 0)
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
			Timestamp: ts,
			Line:      "line",
			Labels:    map[string]string{"job": "test"},
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
		Record{Timestamp: ts, Line: "s1",
			Labels: map[string]string{"job": "test"}},
		Record{Timestamp: ts, Line: "s2",
			Labels: map[string]string{"job": "test"}},
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
		Timestamp: time.Now(),
		Line:      "timer flush",
		Labels:    map[string]string{"job": "test"},
	})
	c.Assert(err, tc.ErrorIsNil)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 1)
	c.Check(p.Streams[0].Values[0][1], tc.Equals, "timer flush")
}

func (s *clientSuite) TestPushGroupsByLabels(c *tc.C) {
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
			Timestamp: ts,
			Line:      "a1",
			Labels:    map[string]string{"job": "a"},
		},
		Record{
			Timestamp: ts,
			Line:      "b1",
			Labels:    map[string]string{"job": "b"},
		},
		Record{
			Timestamp: ts,
			Line:      "a2",
			Labels:    map[string]string{"job": "a"},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 2)
	c.Check(p.Streams[0].Stream["job"], tc.Equals, "a")
	c.Check(p.Streams[0].Values, tc.HasLen, 2)
	c.Check(p.Streams[1].Stream["job"], tc.Equals, "b")
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
			Timestamp: ts,
			Line:      "line",
			Labels:    map[string]string{"job": "test"},
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	// Wait for both HTTP requests to arrive.
	for range 2 {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
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
		Timestamp: time.Now(),
		Line:      "retry me",
		Labels:    map[string]string{"job": "test"},
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
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
		Timestamp: time.Now(),
		Line:      "throttled",
		Labels:    map[string]string{"job": "test"},
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
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
		Timestamp: time.Now(),
		Line:      "will fail",
		Labels:    map[string]string{"job": "test"},
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case pushErr := <-errCh:
		c.Check(
			pushErr, tc.ErrorMatches,
			".*loki returned status 500",
		)
	case <-time.After(5 * time.Second):
		c.Fatal("timed out waiting for OnError callback")
	}
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
			Timestamp: ts,
			Line:      "drain me",
			Labels:    map[string]string{"job": "test"},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	killAndWait(c, client)

	p := waitPayload(c, payloads)
	c.Assert(p.Streams, tc.HasLen, 1)
	c.Check(
		p.Streams[0].Values[0][1],
		tc.Equals,
		"drain me",
	)
}

func (s *clientSuite) TestPushNilLabels(c *tc.C) {
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

func (s *clientSuite) TestBuildPayload(c *tc.C) {
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	records := []Record{
		{
			Timestamp: ts,
			Line:      "a1",
			Labels: map[string]string{
				"job": "x", "env": "prod",
			},
		},
		{
			Timestamp: ts.Add(time.Second),
			Line:      "b1",
			Labels:    map[string]string{"job": "y"},
		},
		{
			Timestamp: ts.Add(2 * time.Second),
			Line:      "a2",
			Labels: map[string]string{
				"env": "prod", "job": "x",
			},
		},
	}

	payload := buildPayload(records)
	c.Assert(payload.Streams, tc.HasLen, 2)

	c.Check(
		payload.Streams[0].Stream,
		tc.DeepEquals,
		map[string]string{"job": "x", "env": "prod"},
	)
	c.Check(payload.Streams[0].Values, tc.HasLen, 2)
	c.Check(payload.Streams[0].Values[0][1], tc.Equals, "a1")
	c.Check(payload.Streams[0].Values[1][1], tc.Equals, "a2")

	c.Check(
		payload.Streams[1].Stream,
		tc.DeepEquals,
		map[string]string{"job": "y"},
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
		FlushInterval:  50 * time.Millisecond,
		MaxRetries:     3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Clock:          clock.WallClock,
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
	case <-time.After(5 * time.Second):
		c.Fatal("timed out waiting for payload")
		return pushPayload{}
	}
}

func killAndWait(c *tc.C, client *Client) {
	client.Kill(nil)
	err := client.Wait()
	c.Assert(err, tc.ErrorIsNil)
}
