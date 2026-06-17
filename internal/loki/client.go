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
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"gopkg.in/tomb.v2"

	coreerrors "github.com/juju/juju/core/errors"
	internalerrors "github.com/juju/juju/internal/errors"
)

// Record represents a single log entry to push to Loki.
type Record struct {
	// Timestamp is when the log entry was produced.
	Timestamp time.Time

	// Line is the log message text.
	Line string

	// ControllerUUID is the Juju controller UUID for topology labeling.
	ControllerUUID string

	// ModelUUID is the Juju model UUID for topology labeling.
	ModelUUID string

	// AgentID is the stable Juju agent identity for topology labeling.
	AgentID string

	// Fields are structured log fields. High-cardinality values belong here,
	// not in Loki labels.
	Fields map[string]string

	// TraceID is the OpenTelemetry trace ID for this record, if present.
	TraceID string

	// SpanID is the OpenTelemetry span ID for this record, if present.
	SpanID string
}

// HTTPClient is the HTTP client surface used by the Loki push client.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Config holds configuration for the Loki push client.
type Config struct {
	// BatchSize is the maximum number of records to
	// accumulate before flushing to Loki. Default: 100.
	BatchSize int

	// BufferSize is the maximum number of queued records waiting to be
	// batched. When full, Push drops the oldest queued records. Default: 500.
	BufferSize int

	// FlushInterval is how long to wait before flushing
	// buffered records even if BatchSize hasn't been
	// reached. Default: 10s.
	FlushInterval time.Duration

	// MaxRetries is the maximum number of retry attempts
	// after the initial push request. Set to 0 to disable
	// retries. Default: 3.
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
	HTTPClient HTTPClient

	// AsyncFlush controls whether batches are pushed in a
	// background goroutine. When true (the default), the
	// loop continues consuming records while HTTP I/O
	// happens concurrently. When false, each flush blocks
	// the loop until the push completes.
	AsyncFlush *bool

	// OnError is called when a push fails after all retry
	// attempts are exhausted. If nil, no callback is invoked.
	OnError func(error)

	// OnDrop is called when records are dropped from the circular buffer. If
	// nil, no callback is invoked.
	OnDrop func(int)

	// Clock is passed to the retry logic for testing.
	// If nil, the wall clock is used.
	Clock clock.Clock
}

// Validate checks that the Config has valid values and returns an
// error if any field is invalid.
func (c Config) Validate() error {
	if c.BatchSize <= 0 {
		return internalerrors.Errorf("BatchSize must be positive")
	}
	if c.BufferSize <= 0 {
		return internalerrors.Errorf("BufferSize must be positive")
	}
	if c.FlushInterval <= 0 {
		return internalerrors.Errorf("FlushInterval must be positive")
	}
	if c.MaxRetries < 0 {
		return internalerrors.Errorf("MaxRetries must not be negative")
	}
	if c.InitialBackoff <= 0 {
		return internalerrors.Errorf("InitialBackoff must be positive")
	}
	if c.MaxBackoff <= 0 {
		return internalerrors.Errorf("MaxBackoff must be positive")
	}
	if c.Clock == nil {
		return internalerrors.Errorf("Clock must not be nil")
	}
	return nil
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BatchSize:      100,
		BufferSize:     500,
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
	http       HTTPClient
	asyncFlush bool
	onError    func(error)
	onDrop     func(int)
	records    chan Record
	stats      report
	tomb       tomb.Tomb
}

type report struct {
	Dropped    uint64
	Sent       uint64
	PushErrors uint64
}

// NewClient creates and starts a new Loki push client worker.
// The endpoint should be the full URL to the Loki push API
// (e.g. http://loki:3100/loki/api/v1/push).
func NewClient(endpoint string, cfg Config) (*Client, error) {
	if endpoint == "" {
		return nil, internalerrors.Errorf("endpoint must not be empty").Add(coreerrors.NotValid)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	c := &Client{
		endpoint:   endpoint,
		cfg:        cfg,
		http:       cfg.HTTPClient,
		asyncFlush: cfg.AsyncFlush == nil || *cfg.AsyncFlush,
		onError:    cfg.OnError,
		onDrop:     cfg.OnDrop,
		records:    make(chan Record, cfg.BufferSize),
	}
	c.tomb.Go(c.loop)
	return c, nil
}

// Push sends records to the client for delivery to Loki.
// Records are buffered internally and flushed when the batch
// size is reached or the flush interval elapses. Push does not
// block on a full queue; it drops the oldest queued records first.
// Returns tomb.ErrDying if the client is shutting down. Push does
// not deep-copy records, so callers should not mutate record
// contents after calling this method.
func (c *Client) Push(records ...Record) error {
	for _, r := range records {
		if err := c.pushOne(r); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) pushOne(r Record) error {
	for {
		select {
		case c.records <- r:
			return nil
		case <-c.tomb.Dying():
			return tomb.ErrDying
		default:
		}

		select {
		case <-c.records:
			atomic.AddUint64(&c.stats.Dropped, 1)
			c.notifyDrop(1)
		case <-c.tomb.Dying():
			return tomb.ErrDying
		default:
		}
	}
}

// Report implements worker.Reporter.
func (c *Client) Report(context.Context) map[string]any {
	return map[string]any{
		"dropped":     atomic.LoadUint64(&c.stats.Dropped),
		"sent":        atomic.LoadUint64(&c.stats.Sent),
		"push-errors": atomic.LoadUint64(&c.stats.PushErrors),
	}
}

// Kill requests the client to stop. Any buffered records are
// flushed on a best-effort basis before the client exits.
// Records in-flight during shutdown may be dropped.
func (c *Client) Kill() {
	c.tomb.Kill(nil)
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
				func() {
					// Don't use the tomb context here, because we want to give
					// the flush a chance to complete even if the tomb is dying.
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					c.pushAll(ctx, buffer)
				}()
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
	batchCopy := make([]Record, len(batch))
	copy(batchCopy, batch)

	go func() {
		select {
		case <-c.tomb.Dying():
			return
		case <-ctx.Done():
			return
		default:
		}
		c.pushAll(ctx, batchCopy)
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
			atomic.AddUint64(&c.stats.PushErrors, 1)
			c.notifyError(err)
			continue
		}
		atomic.AddUint64(&c.stats.Sent, uint64(end-i))
	}
}

func (c *Client) notifyDrop(count int) {
	if c.onDrop != nil {
		c.onDrop(count)
	}
}

func (c *Client) notifyError(err error) {
	if c.onError != nil {
		c.onError(err)
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

	attempts := c.cfg.MaxRetries + 1
	err = retry.Call(retry.CallArgs{
		Attempts: attempts,
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
	defer func() {
		_ = resp.Body.Close()
	}()

	// Read and discard the body to enable connection reuse.
	if _, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024)); err != nil {
		return &retryableError{
			msg: fmt.Sprintf("reading response: %s", err),
		}
	}

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
	Values []pushValue       `json:"values"`
}

type pushValue struct {
	Timestamp string
	Line      string
	Fields    map[string]string
}

func (v pushValue) MarshalJSON() ([]byte, error) {
	if len(v.Fields) == 0 {
		return json.Marshal([]string{v.Timestamp, v.Line})
	}
	return json.Marshal([]any{v.Timestamp, v.Line, v.Fields})
}

func (v *pushValue) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 2 && len(raw) != 3 {
		return internalerrors.Errorf("expected loki value tuple with 2 or 3 fields")
	}
	if err := json.Unmarshal(raw[0], &v.Timestamp); err != nil {
		return err
	}
	if err := json.Unmarshal(raw[1], &v.Line); err != nil {
		return err
	}
	if len(raw) == 3 {
		if err := json.Unmarshal(raw[2], &v.Fields); err != nil {
			return err
		}
	}
	return nil
}

// buildPayload groups records by label set into streams.
func buildPayload(records []Record) pushPayload {
	groups := make(map[string]*pushStream)
	order := make([]string, 0)

	for _, r := range records {
		labels := topologyLabels(r)
		key := labelKey(labels)
		s, ok := groups[key]
		if !ok {
			s = &pushStream{Stream: labels}
			groups[key] = s
			order = append(order, key)
		}
		s.Values = append(s.Values, pushValue{
			Timestamp: strconv.FormatInt(r.Timestamp.UnixNano(), 10),
			Line:      r.Line,
			Fields:    structuredFields(r),
		})
	}

	streams := make([]pushStream, 0, len(order))
	for _, key := range order {
		streams = append(streams, *groups[key])
	}
	return pushPayload{Streams: streams}
}

func topologyLabels(r Record) map[string]string {
	labels := make(map[string]string, 3)
	if r.ControllerUUID != "" {
		labels["juju_controller"] = r.ControllerUUID
	}
	if r.ModelUUID != "" {
		labels["juju_model"] = r.ModelUUID
	}
	if r.AgentID != "" {
		labels["juju_agent"] = r.AgentID
	}
	return labels
}

func structuredFields(r Record) map[string]string {
	fields := make(map[string]string, len(r.Fields)+2)
	maps.Copy(fields, r.Fields)
	if isLowerHex(r.TraceID, 32) {
		fields["trace_id"] = r.TraceID
	}
	if isLowerHex(r.SpanID, 16) {
		fields["span_id"] = r.SpanID
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func isLowerHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, r := range s {
		if !('0' <= r && r <= '9') && !('a' <= r && r <= 'f') {
			return false
		}
	}
	return true
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
