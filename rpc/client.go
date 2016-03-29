// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"strings"
	"sync/atomic"

	"github.com/juju/errors"
)

var ErrShutdown = errors.New("connection is shut down")

func IsShutdownErr(err error) bool {
	return errors.Cause(err) == ErrShutdown
}

// Call represents an active RPC.
type Call struct {
	Request
	Params   interface{}
	Response interface{}
	Error    error
	Done     chan *Call
}

// RequestError represents an error returned from an RPC request.
type RequestError struct {
	Message string
	Code    string
}

func (e *RequestError) Error() string {
	if e.Code != "" {
		return e.Message + " (" + e.Code + ")"
	}
	return e.Message
}

func (e *RequestError) ErrorCode() string {
	return e.Code
}

// ClientConn represents a RPC client connection.
type ClientConn interface {
	// Call invokes the named action on the object of the given type with the given
	// id. The returned values will be stored in response, which should be a pointer.
	// If the action fails remotely, the error will have a cause of type RequestError.
	// The params value may be nil if no parameters are provided; the response value
	// may be nil to indicate that any result should be discarded.
	Call(req Request, params, response interface{}) error

	// Close closes the connection.
	Close() error
}

// NewClientConn returns a ClientConn for the underlying codec.
func NewClientConn(codec Codec) ClientConn {
	var notifier dummyNotifier
	client := newConn(codec, &notifier)
	client.Start()
	return client
}

func (c *conn) send(call *Call) {
	c.sending.Lock()
	defer c.sending.Unlock()

	// Register this call.
	c.mutex.Lock()
	if c.dead == nil {
		panic("rpc: call made when connection not started")
	}
	if c.closing || c.shutdown {
		call.Error = ErrShutdown
		c.mutex.Unlock()
		call.done()
		return
	}
	reqId := atomic.AddUint64(&c.reqId, 1)
	c.clientPending[reqId] = call
	c.mutex.Unlock()

	// Encode and send the request.
	hdr := &Header{
		RequestId: reqId,
		Request:   call.Request,
	}
	params := call.Params
	if params == nil {
		params = struct{}{}
	}
	if c.notifier != nil {
		c.notifier.ClientRequest(hdr, params)
	}
	if err := c.codec.WriteMessage(hdr, params); err != nil {
		c.mutex.Lock()
		call = c.clientPending[reqId]
		delete(c.clientPending, reqId)
		c.mutex.Unlock()
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (c *conn) handleResponse(hdr *Header) error {
	reqId := hdr.RequestId
	c.mutex.Lock()
	call := c.clientPending[reqId]
	delete(c.clientPending, reqId)
	c.mutex.Unlock()

	var err error
	switch {
	case call == nil:
		// We've got no pending call. That usually means that
		// WriteHeader partially failed, and call was already
		// removed; response is a server telling us about an
		// error reading request body. We should still attempt
		// to read error body, but there's no one to give it to.
		c.notifier.ClientReply(Request{}, hdr, nil)
		err = c.readBody(nil, false)
	case hdr.Error != "":
		// Report rpcreflect.NoSuchMethodError with CodeNotImplemented.
		if strings.HasPrefix(hdr.Error, "no such request ") && hdr.ErrorCode == "" {
			hdr.ErrorCode = codeNotImplemented
		}
		// We've got an error response. Give this to the request;
		// any subsequent requests will get the ReadResponseBody
		// error if there is one.
		call.Error = &RequestError{
			Message: hdr.Error,
			Code:    hdr.ErrorCode,
		}
		err = c.readBody(nil, false)
		c.notifier.ClientReply(call.Request, hdr, nil)
		call.done()
	default:
		err = c.readBody(call.Response, false)
		c.notifier.ClientReply(call.Request, hdr, call.Response)
		call.done()
	}
	return errors.Annotate(err, "error handling response")
}

func (call *Call) done() {
	select {
	case call.Done <- call:
		// ok
	default:
		// We don't want to block here.  It is the caller's responsibility to make
		// sure the channel has enough buffer space. See comment in Go().
		logger.Errorf("discarding Call reply due to insufficient Done chan capacity")
	}
}

// Call invokes the named action on the object of the given type with the given
// id. The returned values will be stored in response, which should be a pointer.
// If the action fails remotely, the error will have a cause of type RequestError.
// The params value may be nil if no parameters are provided; the response value
// may be nil to indicate that any result should be discarded.
func (c *conn) Call(req Request, params, response interface{}) error {
	call := &Call{
		Request:  req,
		Params:   params,
		Response: response,
		Done:     make(chan *Call, 1),
	}
	c.send(call)
	result := <-call.Done
	return errors.Trace(result.Error)
}
