// Copyright 2015-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"io"
	"net/http"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/ratelimit"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/featureflag"
	"github.com/juju/version"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/websocket"
	"github.com/juju/juju/feature"
)

var logger = loggo.GetLogger("juju.apiserver.logsink")

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
) http.Handler {
	return &logSinkHandler{
		newLogWriteCloser: newLogWriteCloser,
		abort:             abort,
		ratelimit:         ratelimit,
	}
}

type logSinkHandler struct {
	newLogWriteCloser NewLogWriteCloserFunc
	abort             <-chan struct{}
	ratelimit         *RateLimitConfig
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
	handler := func(socket *websocket.Conn) {
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
			socket.SetReadDeadline(time.Now().Add(websocket.PongDelay))
			socket.SetPongHandler(func(string) error {
				logger.Tracef("pong logsink %p", socket)
				socket.SetReadDeadline(time.Now().Add(websocket.PongDelay))
				return nil
			})
			ticker := time.NewTicker(websocket.PingPeriod)
			defer ticker.Stop()
			tickChannel = ticker.C
		} else {
			socket.SetReadDeadline(time.Now().Add(vZeroDelay))
		}

		logCh := h.receiveLogs(socket, endpointVersion)
		for {
			select {
			case <-h.abort:
				return
			case <-tickChannel:
				deadline := time.Now().Add(websocket.WriteWait)
				logger.Tracef("ping logsink %p", socket)
				if err := socket.WriteControl(gorillaws.PingMessage, []byte{}, deadline); err != nil {
					// This error is expected if the other end goes away. By
					// returning we clean up the strategy and close the socket
					// through the defer calls.
					logger.Debugf("failed to write ping: %s", err)
					return
				}
			case m, ok := <-logCh:
				if !ok {
					return
				}
				if err := writer.WriteLog(m); err != nil {
					h.sendError(socket, req, err)
					return
				}
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

func (h *logSinkHandler) receiveLogs(socket *websocket.Conn, endpointVersion int) <-chan params.LogRecord {
	logCh := make(chan params.LogRecord)

	var tokenBucket *ratelimit.Bucket
	if h.ratelimit != nil {
		tokenBucket = ratelimit.NewBucketWithClock(
			h.ratelimit.Refill,
			h.ratelimit.Burst,
			ratelimitClock{h.ratelimit.Clock},
		)
	}

	go func() {
		// Close the channel to signal ServeHTTP to finish. Otherwise
		// we leak goroutines on client disconnect, because the server
		// isn't shutting down so h.abort is never closed.
		defer close(logCh)
		var m params.LogRecord
		for {
			// Receive() blocks until data arrives but will also be
			// unblocked when the API handler calls socket.Close as it
			// finishes.
			if err := socket.ReadJSON(&m); err != nil {
				if gorillaws.IsUnexpectedCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
					logger.Debugf("logsink receive error: %v", err)
				} else {
					logger.Debugf("disconnected, %p", socket)
				}
				// Try to tell the other end we are closing. If the other end
				// has already disconnected from us, this will fail, but we don't
				// care that much.
				socket.WriteMessage(gorillaws.CloseMessage, []byte{})
				return
			}

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
				return
			case logCh <- m:
				// If the remote end does not support ping/pong, we bump
				// the read deadline everytime a message is received.
				if endpointVersion == 0 {
					socket.SetReadDeadline(time.Now().Add(vZeroDelay))
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
	if err != nil && featureflag.Enabled(feature.DeveloperMode) {
		logger.Errorf("returning error from %s %s: %s", req.Method, req.URL.Path, errors.Details(err))
	}
	if sendErr := ws.SendInitialErrorV0(err); sendErr != nil {
		logger.Errorf("closing websocket, %v", err)
		ws.Close()
	}
}

// JujuClientVersionFromRequest returns the Juju client version
// number from the HTTP request.
func JujuClientVersionFromRequest(req *http.Request) (version.Number, error) {
	verStr := req.URL.Query().Get("jujuclientversion")
	if verStr == "" {
		return version.Zero, errors.New(`missing "jujuclientversion" in URL query`)
	}
	ver, err := version.Parse(verStr)
	if err != nil {
		return version.Zero, errors.Annotatef(err, "invalid jujuclientversion %q", verStr)
	}
	return ver, nil
}

// ratelimitClock adapts clock.Clock to ratelimit.Clock.
type ratelimitClock struct {
	clock.Clock
}

// Sleep is defined by the ratelimit.Clock interface.
func (c ratelimitClock) Sleep(d time.Duration) {
	<-c.Clock.After(d)
}
