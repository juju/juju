// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package loki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"gopkg.in/tomb.v2"

	internalerrors "github.com/juju/juju/internal/errors"
)

// Record represents a single log entry to push to Loki.
type Record struct {
	// Timestamp is when the log entry was produced.
	Timestamp time.Time

	// Line is the log message text.
	Line string

	// Labels are the Loki stream labels for this entry.
	// Records with identical label sets are grouped into
	// the same stream.
	Labels map[string]string
}

// Config holds configuration for the Loki push client.
type Config struct {
	// BatchSize is the maximum number of records to
	// accumulate before flushing to Loki. Default: 100.
	BatchSize int

	// FlushInterval is how long to wait before flushing
	// buffered records even if BatchSize hasn't been
	// reached. Default: 10s.
	FlushInterval time.Duration

	// MaxRetries is the maximum number of retry attempts
	// for a failed push request. Default: 3.
	MaxRetries int

	// InitialBackoff is the delay before the first retry.
	// Subsequent retries double this value up to MaxBackoff.
	// Default: 500ms.
	InitialBackoff time.Duration

	// MaxBackoff is the maximum delay between retries.
	// Default: 30s.
	MaxBackoff time.Duration

	// HTTPClient is an optional HTTP client. If nil, a
	// default client with a 10s timeout is used.
	HTTPClient *http.Client

	// AsyncFlush controls whether batches are pushed in a
	// background goroutine. When true (the default), the
	// loop continues consuming records while HTTP I/O
	// happens concurrently. When false, each flush blocks
	// the loop until the push completes.
	AsyncFlush *bool

	// OnError is called when a push fails after all retry
	// attempts are exhausted. If nil, errors are silently
	// dropped. This can be used to write to stderr or any
	// other error reporting mechanism.
	OnError func(error)

	// Clock is passed to the retry logic for testing. If nil, the wall clock is
	// used.
	Clock clock.Clock
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BatchSize:      100,
		FlushInterval:  10 * time.Second,
		MaxRetries:     3,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     30 * time.Second,
		Clock:          clock.WallClock,
	}
}

// Client is a worker that buffers log records and pushes them
// to a Loki endpoint. It flushes when the buffer reaches
// BatchSize or when FlushInterval elapses, whichever comes
// first. Flushes are asynchronous by default but can be made
// synchronous via the AsyncFlush configuration option.
type Client struct {
	endpoint   string
	cfg        Config
	http       *http.Client
	asyncFlush bool
	onError    func(error)
	records    chan Record
	tomb       tomb.Tomb
}

// NewClient creates and starts a new Loki push client worker.
// The endpoint should be the full URL to the Loki push API
// (e.g. http://loki:3100/loki/api/v1/push).
// Zero-value fields in cfg are replaced with defaults.
func NewClient(endpoint string, cfg Config) (*Client, error) {
	if endpoint == "" {
		return nil, internalerrors.Errorf("endpoint must not be empty")
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 10 * time.Second
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 3
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	onError := cfg.OnError
	if onError == nil {
		onError = func(error) {}
	}
	asyncFlush := cfg.AsyncFlush == nil || *cfg.AsyncFlush
	c := &Client{
		endpoint:   endpoint,
		cfg:        cfg,
		http:       httpClient,
		asyncFlush: asyncFlush,
		onError:    onError,
		records:    make(chan Record, cfg.BatchSize),
	}
	c.tomb.Go(c.loop)
	return c, nil
}

// Push sends records to the client for delivery to Loki.
// Records are buffered internally and flushed when the batch
// size is reached or the flush interval elapses. Push blocks
// if the internal channel is full, providing backpressure to
// the caller. Returns tomb.ErrDying if the client is shutting
// down.
func (c *Client) Push(records ...Record) error {
	for _, r := range records {
		select {
		case c.records <- r:
		case <-c.tomb.Dying():
			return tomb.ErrDying
		}
	}
	return nil
}

// Kill requests the client to stop. Any buffered records are
// flushed on a best-effort basis before the client exits.
func (c *Client) Kill(reason error) {
	c.tomb.Kill(reason)
}

// Wait blocks until the client has stopped.
func (c *Client) Wait() error {
	return c.tomb.Wait()
}

// loop is the main worker loop. It accumulates records from
// the channel into a buffer, flushing when the batch size is
// reached or the flush interval timer fires. On shutdown, it
// drains any remaining records from the channel and performs
// a best-effort flush with a timeout.
func (c *Client) loop() error {
	buffer := make([]Record, 0, c.cfg.BatchSize)
	timer := time.NewTimer(c.cfg.FlushInterval)
	defer timer.Stop()

	ctx := c.tomb.Context(context.Background())

	for {
		select {
		case <-c.tomb.Dying():
			c.drainChannel(&buffer)
			if len(buffer) > 0 {
				ctx, cancel := context.WithTimeout(
					context.Background(), 5*time.Second,
				)
				defer cancel()
				c.pushAll(ctx, buffer)
			}
			return tomb.ErrDying

		case r := <-c.records:
			buffer = append(buffer, r)
			if len(buffer) >= c.cfg.BatchSize {
				c.flush(ctx, buffer)
				buffer = make([]Record, 0, c.cfg.BatchSize)
				resetTimer(timer, c.cfg.FlushInterval)
			}

		case <-timer.C:
			if len(buffer) > 0 {
				c.flush(ctx, buffer)
				buffer = make([]Record, 0, c.cfg.BatchSize)
			}
			timer.Reset(c.cfg.FlushInterval)
		}
	}
}

// drainChannel reads remaining records from the channel into
// the buffer.
func (c *Client) drainChannel(buffer *[]Record) {
	for {
		select {
		case r := <-c.records:
			*buffer = append(*buffer, r)
		default:
			return
		}
	}
}

// flush dispatches the batch either synchronously or
// asynchronously based on the AsyncFlush configuration.
func (c *Client) flush(ctx context.Context, batch []Record) {
	if c.asyncFlush {
		c.flushAsync(ctx, batch)
		return
	}
	c.pushAll(ctx, batch)
}

// flushAsync sends the batch to Loki in a background
// goroutine. If the tomb is dying or the context is
// cancelled before the goroutine starts, the push is
// abandoned.
func (c *Client) flushAsync(ctx context.Context, batch []Record) {
	go func() {
		select {
		case <-c.tomb.Dying():
			return
		case <-ctx.Done():
			return
		default:
		}
		c.pushAll(ctx, batch)
	}()
}

// pushAll pushes all records to Loki, splitting into
// BatchSize chunks. Push errors are reported via the
// OnError callback.
func (c *Client) pushAll(
	ctx context.Context, records []Record,
) {
	for i := 0; i < len(records); i += c.cfg.BatchSize {
		end := min(i+c.cfg.BatchSize, len(records))
		if err := c.pushBatch(ctx, records[i:end]); err != nil {
			c.onError(err)
		}
	}
}

// resetTimer safely resets a timer, draining the channel
// if the timer has already fired.
func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

// pushBatch marshals records into a Loki push payload and
// sends it to the endpoint. Failed requests are retried
// with exponential backoff.
func (c *Client) pushBatch(
	ctx context.Context, records []Record,
) error {
	payload := buildPayload(records)
	data, err := json.Marshal(payload)
	if err != nil {
		return internalerrors.Errorf("marshaling payload: %w", err)
	}

	err = retry.Call(retry.CallArgs{
		Attempts: c.cfg.MaxRetries,
		Delay:    c.cfg.InitialBackoff,
		MaxDelay: c.cfg.MaxBackoff,
		Func: func() error {
			return c.doRequest(ctx, data)
		},
		IsFatalError: func(err error) bool {
			return !isRetryable(err)
		},
		BackoffFunc: retry.ExpBackoff(c.cfg.InitialBackoff, c.cfg.MaxBackoff, 2.0, true),
		Clock:       c.cfg.Clock,
		Stop:        ctx.Done(),
	})
	if retry.IsAttemptsExceeded(err) {
		return retry.LastError(err)
	}
	if retry.IsRetryStopped(err) {
		return ctx.Err()
	}
	return err
}

// retryableError indicates a push request failure that can
// be retried.
type retryableError struct {
	msg string
}

func (e *retryableError) Error() string {
	return e.msg
}

// isRetryable reports whether err is a retryableError.
func isRetryable(err error) bool {
	_, ok := err.(*retryableError)
	return ok
}

// doRequest sends a single HTTP POST to the Loki push
// endpoint. Returns a retryableError for transient failures
// (network errors, 429, 5xx) or a plain error for
// non-retryable status codes.
func (c *Client) doRequest(
	ctx context.Context, data []byte,
) error {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.endpoint, bytes.NewReader(data),
	)
	if err != nil {
		return internalerrors.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return &retryableError{
			msg: fmt.Sprintf("sending request: %s", err),
		}
	}
	// Read and discard the body to enable connection reuse.
	if _, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024)); err != nil {
		return &retryableError{
			msg: fmt.Sprintf("reading response: %s", err),
		}
	}
	_ = resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNoContent ||
		resp.StatusCode == http.StatusOK:
		return nil

	case resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= http.StatusInternalServerError:
		return &retryableError{
			msg: fmt.Sprintf(
				"loki returned status %d", resp.StatusCode,
			),
		}

	default:
		return internalerrors.Errorf(
			"loki returned status %d", resp.StatusCode,
		)
	}
}

// pushPayload is the JSON structure for the Loki push API.
type pushPayload struct {
	Streams []pushStream `json:"streams"`
}

// pushStream is a single stream within a push payload.
type pushStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// buildPayload groups records by label set into streams.
func buildPayload(records []Record) pushPayload {
	groups := make(map[string]*pushStream)
	order := make([]string, 0)

	for _, r := range records {
		key := labelKey(r.Labels)
		s, ok := groups[key]
		if !ok {
			labels := make(map[string]string, len(r.Labels))
			maps.Copy(labels, r.Labels)

			s = &pushStream{Stream: labels}
			groups[key] = s
			order = append(order, key)
		}
		ts := strconv.FormatInt(
			r.Timestamp.UnixNano(), 10,
		)
		s.Values = append(s.Values, []string{ts, r.Line})
	}

	streams := make([]pushStream, 0, len(order))
	for _, key := range order {
		streams = append(streams, *groups[key])
	}
	return pushPayload{Streams: streams}
}

// labelKey produces a deterministic string key from a label
// map for grouping records into streams.
func labelKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}
