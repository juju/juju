// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

const CodeNotImplemented = codeNotImplemented

// TODO(katco): Remove this as it is exposing internal state of Conn. Age old story: ran out of time to rewrite the tests to do this correctly.

// ClientRequestID exposes the client's request ID which is
// incremented everytime a connection sends a request.
func (c *Conn) ClientRequestID() uint64 {
	return c.reqId
}
