// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import "time"

const CodeNotImplemented = codeNotImplemented

// TODO(katco): Remove this as it is exposing internal state of Conn. Age old story: ran out of time to rewrite the tests to do this correctly.

// ClientRequestID exposes the client's request ID which is
// incremented everytime a connection sends a request.
func (c *Conn) ClientRequestID() uint64 {
	return c.reqId.Load()
}

// SetCloseTimeout overrides the timeout Close waits for outstanding
// server requests to complete. Intended for tests only.
func (c *Conn) SetCloseTimeout(d time.Duration) {
	c.closeTimeout = d
}

// SetWriteFlushTimeout overrides the timeout Close waits for queued
// responses to be written. Intended for tests only.
func (c *Conn) SetWriteFlushTimeout(d time.Duration) {
	c.writeFlushTimeout = d
}

// WaitForPendingServerRequests waits for all server request goroutines to
// return. Intended for tests only.
func (c *Conn) WaitForPendingServerRequests() {
	c.srvPending.Wait()
}

// PendingResponseCount returns the number of responses being queued or
// written. Intended for tests only.
func (c *Conn) PendingResponseCount() int64 {
	return c.pendingWrites.n.Load()
}
