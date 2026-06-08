// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	stderrors "errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/tc"
)

type transientLogDeliveryErrorSuite struct{}

func TestTransientLogDeliveryErrorSuite(t *testing.T) {
	tc.Run(t, &transientLogDeliveryErrorSuite{})
}

func (s *transientLogDeliveryErrorSuite) TestTransientErrors(c *tc.C) {
	tests := []struct {
		name string
		err  error
	}{{
		name: "EOF",
		err:  io.EOF,
	}, {
		name: "wrapped EOF",
		err:  errors.Annotate(io.EOF, "sending log message"),
	}, {
		name: "websocket normal close",
		err:  &gorillaws.CloseError{Code: gorillaws.CloseNormalClosure},
	}, {
		name: "websocket going away",
		err:  &gorillaws.CloseError{Code: gorillaws.CloseGoingAway},
	}, {
		name: "websocket no status",
		err:  &gorillaws.CloseError{Code: gorillaws.CloseNoStatusReceived},
	}, {
		name: "websocket abnormal close",
		err:  &gorillaws.CloseError{Code: gorillaws.CloseAbnormalClosure},
	}, {
		name: "websocket unexpected close",
		err:  &gorillaws.CloseError{Code: gorillaws.CloseUnsupportedData},
	}, {
		name: "503 status",
		err:  stderrors.New("cannot connect to /logsink: server returned HTTP status 503"),
	}, {
		name: "service unavailable",
		err:  stderrors.New("cannot connect to /logsink: Service Unavailable"),
	}, {
		name: "write failed",
		err:  stderrors.New("sending log message: write failed"),
	}, {
		name: "closed network connection",
		err:  stderrors.New("use of closed network connection"),
	}, {
		name: "disconnected api caller",
		err:  stderrors.New("api caller disconnected"),
	}, {
		name: "network timeout",
		err: &net.DNSError{
			Err:         "i/o timeout",
			IsTimeout:   true,
			IsTemporary: true,
		},
	}, {
		name: "connection reset",
		err:  syscall.ECONNRESET,
	}, {
		name: "broken pipe",
		err:  syscall.EPIPE,
	}}

	for _, test := range tests {
		c.Check(isTransientLogDeliveryError(test.err), tc.IsTrue, tc.Commentf(test.name))
	}
}

func (s *transientLogDeliveryErrorSuite) TestNonTransientErrors(c *tc.C) {
	tests := []struct {
		name string
		err  error
	}{{
		name: "nil",
		err:  nil,
	}, {
		name: "unknown error",
		err:  stderrors.New("permission denied"),
	}, {
		name: "context deadline",
		err:  contextDeadlineError{},
	}}

	for _, test := range tests {
		c.Check(isTransientLogDeliveryError(test.err), tc.IsFalse, tc.Commentf(test.name))
	}
}

type contextDeadlineError struct{}

func (contextDeadlineError) Error() string {
	return "context deadline exceeded"
}

func (contextDeadlineError) Timeout() bool {
	return false
}

func (contextDeadlineError) Temporary() bool {
	return false
}

func (contextDeadlineError) Deadline() (time.Time, bool) {
	return time.Time{}, false
}
