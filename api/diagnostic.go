// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"net"
	"runtime/debug"
	"sync/atomic"

	"github.com/juju/loggo/v2"
)

var connCount int64

// trackedConn wraps a [net.Conn] so that we can track its creation and closure.
// See [WrapDialContext] below for usage.
type trackedConn struct {
	net.Conn

	createdStack []byte
	closed       int32
	id           int64

	logger loggo.Logger
}

func newTrackedConn(c net.Conn, addr string) *trackedConn {
	tc := &trackedConn{
		Conn:         c,
		createdStack: debug.Stack(),
		id:           atomic.AddInt64(&connCount, 1),
		logger:       loggo.GetLogger("juju.api.diagnostic"),
	}

	tc.logger.Criticalf("[diagnostic] Opened conn id=%d to %s; created by:\n%s\n", tc.id, addr, string(tc.createdStack))
	return tc
}

func (t *trackedConn) Close() error {
	if atomic.CompareAndSwapInt32(&t.closed, 0, 1) {
		t.logger.Criticalf("[diagnostic] Closing conn id=%d created by::\n%s\n", t.id, string(t.createdStack))
	} else {
		t.logger.Criticalf("[diagnostic] Close called again id=%d\n", t.id)
	}
	return t.Conn.Close()
}

// WrapDialContext is a [net.Dialer.DialContext] wrapper.
// It works by wrapping the dialled [net.Conn] with our [trackedConn].
// This is intended for use in debugging builds to observe the call
// stacks that create low-level connections, but do not close them.
func WrapDialContext(
	dial func(ctx context.Context, network, addr string) (net.Conn, error),
) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		tc := newTrackedConn(c, addr)
		return tc, nil
	}
}
