// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/ratelimit"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/websocket"
	"github.com/juju/juju/internal/featureflag"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

const (
	metricLogWriteLabelSuccess = "success"
	metricLogWriteLabelFailure = "failure"
)

const (
	metricLogReadLabelSuccess    = "success"
	metricLogReadLabelError      = "error"
	metricLogReadLabelDisconnect = "disconnect"
)

var logger = internallogger.GetLogger("juju.apiserver.logsink")

// LogWriteCloser provides an interface for persisting log records.
// The LogCloser's Close method should be called to release any
// resources once it is done with.
type LogWriteCloser interface {
	io.Closer

	// WriteLog writes out the given log record.
	WriteLog(params.LogRecord) error
}

// NewLogWriteCloserFunc returns a new LogWriteCloser for the given http.Request.
type NewLogWriteCloserFunc func(*http.Request) (LogWriteCloser, error)

// RateLimitConfig contains the rate-limit configuration for the logsink
// handler.
type RateLimitConfig struct {
	// Burst is the number of log messages that will be let through before
	// we start rate limiting.
	Burst int64

	// Refill is the rate at which log messages will be let through once
	// the initial burst amount has been depleted.
	Refill time.Duration

	// Clock is the clock used to wait when rate-limiting log receives.
	Clock clock.Clock
}

// CounterVec is a Collector that bundles a set of Counters that all share the
// same description.
type CounterVec interface {
	// With returns a Counter for a given labels slice
	With(prometheus.Labels) prometheus.Counter
}

// GaugeVec is a Collector that bundles a set of Gauges that all share the
// same description.
type GaugeVec interface {
	// With returns a Gauge for a given labels slice
	With(prometheus.Labels) prometheus.Gauge
}

// MetricsCollector represents a way to change the metrics for the logsink
// api handler.
//
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/metrics_collector_mock.go github.com/juju/juju/apiserver/logsink MetricsCollector
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/metrics_mock.go github.com/prometheus/client_golang/prometheus Counter,Gauge
type MetricsCollector interface {

	// TotalConnections returns a prometheus metric that can be incremented
	// as a counter for the total number connections being served from the api
	// handler.
	TotalConnections() prometheus.Counter

	// Connections returns a prometheus metric that can be incremented and
	// decremented as a gauge, for the number connections being current served
	// from the api handler.
	Connections() prometheus.Gauge

	// PingFailureCount returns a prometheus metric for the number of
	// ping failures per model uuid, that can be incremented as
	// a counter.
	PingFailureCount(modelUUID string) prometheus.Counter

	// LogWriteCount returns a prometheus metric for the number of writes to
	// the log that happened. It's split on the success/failure, so the charts
	// will have to take that into account.
	LogWriteCount(modelUUID, state string) prometheus.Counter

	// LogReadCount returns a prometheus metric for the number of reads to
	// the log that happened. It's split on the success/error/disconnect, so
	// the charts will have to take that into account.
	LogReadCount(modelUUID, state string) prometheus.Counter
}

// NewHTTPHandler returns a new http.Handler for receiving log messages over a
// websocket, using the given NewLogWriteCloserFunc to obtain a writer to which
// the log messages will be written.
//
// ratelimit defines an optional rate-limit configuration. If nil, no rate-
// limiting will be applied.
func NewHTTPHandler(
	newLogWriteCloser NewLogWriteCloserFunc,
	abort <-chan struct{},
	ratelimit *RateLimitConfig,
	metrics MetricsCollector,
	modelUUID string,
) http.Handler {
	return &logSinkHandler{
		newLogWriteCloser: newLogWriteCloser,
		abort:             abort,
		ratelimit:         ratelimit,
		newStopChannel: func() (chan struct{}, func()) {
			ch := make(chan struct{})
			return ch, func() { close(ch) }
		},
		metrics:   metrics,
		modelUUID: modelUUID,
	}
}

type logSinkHandler struct {
	newLogWriteCloser NewLogWriteCloserFunc
	abort             <-chan struct{}
	ratelimit         *RateLimitConfig
	metrics           MetricsCollector
	modelUUID         string
	mu                sync.Mutex

	// newStopChannel is overridden in tests so that we can check the
	// goroutine exits when prompted.
	newStopChannel  func() (chan struct{}, func())
	receiverStopped bool
}

// Since the logsink only receives messages, it is possible for the other end
// to disappear without the server noticing. To fix this, we use the
// underlying websocket control messages ping/pong. Periodically the server
// writes a ping, and the other end replies with a pong. Now the tricky bit is
// that it appears in all the examples found on the interweb that it is
// possible for the control message to be sent successfully to something that
// isn't entirely alive, which is why relying on an error return from the
// write call is insufficient to mark the connection as dead. Instead the
// write and read deadlines inherent in the underlying Go networking libraries
// are used to force errors on timeouts. However the underlying network
// libraries use time.Now() to determine whether or not to send errors, so
// using a testing clock here isn't going to work. So we rely on manual
// testing, and what is defined as good practice by the library authors.
//
// Now, in theory, we should be using this ping/pong across all the websockets,
// but that is a little outside the scope of this piece of work.

const (
	// For endpoints that don't support ping/pong (i.e. agents prior to 2.2-beta1)
	// we will time out their connections after six hours of inactivity.
	vZeroDelay = 6 * time.Hour
)

// ServeHTTP implements the http.Handler interface.
func (h *logSinkHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// If the modelUUID from the request is empty, fallback to the one in
	// the logsink handler (controller UUID)
	resolvedModelUUID := h.modelUUID
	if modelUUID, valid := httpcontext.RequestModelUUID(req.Context()); valid {
		resolvedModelUUID = modelUUID
	}

	handler := func(socket *websocket.Conn) {
		h.metrics.TotalConnections().Inc()
		h.metrics.Connections().Inc()
		defer h.metrics.Connections().Dec()

		defer socket.Close()
		endpointVersion, err := h.getVersion(req)
		if err != nil {
			h.sendError(socket, req, err)
			return
		}
		writer, err := h.newLogWriteCloser(req)
		if err != nil {
			h.sendError(socket, req, err)
			return
		}
		defer writer.Close()

		// If we get to here, no more errors to report, so we report a nil
		// error.  This way the first line of the socket is always a json
		// formatted simple error.
		h.sendError(socket, req, nil)

		// Here we configure the ping/pong handling for the websocket so the
		// server can notice when the client goes away. Older versions did not
		// respond to ping control messages, so don't try.
		var tickChannel <-chan time.Time
		if endpointVersion > 0 {
			_ = socket.SetReadDeadline(time.Now().Add(websocket.PongDelay))
			socket.SetPongHandler(func(string) error {
				logger.Tracef(req.Context(), "pong logsink %p", socket)
				_ = socket.SetReadDeadline(time.Now().Add(websocket.PongDelay))
				return nil
			})
			ticker := time.NewTicker(websocket.PingPeriod)
			defer ticker.Stop()
			tickChannel = ticker.C
		} else {
			_ = socket.SetReadDeadline(time.Now().Add(vZeroDelay))
		}

		stopReceiving, closer := h.newStopChannel()
		defer closer()
		logCh := h.receiveLogs(req.Context(), socket, endpointVersion, resolvedModelUUID, stopReceiving)
		for {
			select {
			case <-h.abort:
				return
			case <-tickChannel:
				deadline := time.Now().Add(websocket.WriteWait)
				logger.Tracef(req.Context(), "ping logsink %p", socket)
				if err := socket.WriteControl(gorillaws.PingMessage, []byte{}, deadline); err != nil {
					// This error is expected if the other end goes away. By
					// returning we clean up the strategy and close the socket
					// through the defer calls.
					logger.Debugf(req.Context(), "failed to write ping: %s", err)
					// Bump the ping failure count.
					h.metrics.PingFailureCount(resolvedModelUUID).Inc()
					return
				}
			case m, ok := <-logCh:
				if !ok {
					h.mu.Lock()
					h.receiverStopped = true
					h.mu.Unlock()
					return
				}

				if err := writer.WriteLog(m); err != nil {
					h.sendError(socket, req, err)
					// Increment the number of failure cases per modelUUID, that
					// we where unable to write a log to - note: we won't see
					// why the failure happens, only that it did happen. Maybe
					// we should add a trace log here. Developer mode for send
					// error might help if it was enabled at first ?
					h.metrics.LogWriteCount(resolvedModelUUID, metricLogWriteLabelFailure).Inc()
					return
				}

				// Increment the number of successful modelUUID log writes, so
				// that we can see what's a success over failure case
				h.metrics.LogWriteCount(resolvedModelUUID, metricLogWriteLabelSuccess).Inc()
			}
		}
	}
	websocket.Serve(w, req, handler)
}

func (h *logSinkHandler) getVersion(req *http.Request) (int, error) {
	verStr := req.URL.Query().Get("version")
	switch verStr {
	case "":
		return 0, nil
	case "1":
		return 1, nil
	default:
		return 0, errors.Errorf("unknown version %q", verStr)
	}
}

func (h *logSinkHandler) receiveLogs(
	ctx context.Context,
	socket *websocket.Conn,
	endpointVersion int,
	resolvedModelUUID string,
	stop <-chan struct{},
) <-chan params.LogRecord {
	logCh := make(chan params.LogRecord)

	var tokenBucket *ratelimit.Bucket
	if h.ratelimit != nil {
		tokenBucket = ratelimit.NewBucketWithClock(
			h.ratelimit.Refill,
			h.ratelimit.Burst,
			ratelimitClock{Clock: h.ratelimit.Clock},
		)
	}

	go func() {
		// Close the channel to signal ServeHTTP to finish. Otherwise
		// we leak goroutines on client disconnect, because the server
		// isn't shutting down so h.abort is never closed.
		defer close(logCh)
		for {
			// Receive() blocks until data arrives but will also be
			// unblocked when the API handler calls socket.Close as it
			// finishes.

			// Do not lift the LogRecord outside of the for-loop as any fields
			// with omitempty will not get cleared between iterations. The
			// logsink has to work with different versions of juju we need to
			// ensure that we have consistent data, even at the cost of
			// performance.
			var m params.LogRecord
			if err := socket.ReadJSON(&m); err != nil {
				if gorillaws.IsCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
					logger.Tracef(ctx, "logsink closed: %v", err)
					h.metrics.LogReadCount(resolvedModelUUID, metricLogReadLabelDisconnect).Inc()
				} else if gorillaws.IsUnexpectedCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
					logger.Debugf(ctx, "logsink unexpected close error: %v", err)
					h.metrics.LogReadCount(resolvedModelUUID, metricLogReadLabelError).Inc()
				} else {
					logger.Debugf(ctx, "logsink error: %v", err)
					h.metrics.LogReadCount(resolvedModelUUID, metricLogReadLabelError).Inc()
				}
				// Try to tell the other end we are closing. If the other end
				// has already disconnected from us, this will fail, but we don't
				// care that much.
				h.mu.Lock()
				_ = socket.WriteMessage(gorillaws.CloseMessage, []byte{})
				h.mu.Unlock()
				return
			}
			h.metrics.LogReadCount(resolvedModelUUID, metricLogReadLabelSuccess).Inc()

			// Rate-limit receipt of log messages. We rate-limit
			// each connection individually to prevent one noisy
			// individual from drowning out the others.
			if tokenBucket != nil {
				if d := tokenBucket.Take(1); d > 0 {
					select {
					case <-h.ratelimit.Clock.After(d):
					case <-h.abort:
						return
					}
				}
			}

			// Send the log message.
			select {
			case <-h.abort:
				// The API server is stopping.
				return
			case <-stop:
				// The ServeHTTP handler has stopped.
				return
			case logCh <- m:
				// If the remote end does not support ping/pong, we bump
				// the read deadline everytime a message is received.
				if endpointVersion == 0 {
					_ = socket.SetReadDeadline(time.Now().Add(vZeroDelay))
				}
			}
		}
	}()

	return logCh
}

// sendError sends a JSON-encoded error response.
func (h *logSinkHandler) sendError(ws *websocket.Conn, req *http.Request, err error) {
	// There is no need to log the error for normal operators as there is nothing
	// they can action. This is for developers.
	if err != nil && featureflag.Enabled(featureflag.DeveloperMode) {
		logger.Errorf(req.Context(), "returning error from %s %s: %s", req.Method, req.URL.Path, errors.Details(err))
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if sendErr := ws.SendInitialErrorV0(err); sendErr != nil {
		logger.Errorf(req.Context(), "closing websocket, %v", err)
		ws.Close()
	}
}

// ratelimitClock adapts clock.Clock to ratelimit.Clock.
type ratelimitClock struct {
	clock.Clock
}

// Sleep is defined by the ratelimit.Clock interface.
func (c ratelimitClock) Sleep(d time.Duration) {
	<-c.Clock.After(d)
}
