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

// SetResponseHook installs a channel that is signalled (non-blocking)
// each time sendResponse completes (whether the response was queued or
// dropped). It must be called before Start. Intended for tests only.
func (c *Conn) SetResponseHook(ch chan struct{}) {
	c.responseHook = ch
}
