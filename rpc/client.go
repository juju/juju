// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"strings"

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

func (conn *Conn) send(call *Call) {
	conn.sending.Lock()
	defer conn.sending.Unlock()

	// Register this call.
	conn.mutex.Lock()
	if conn.dead == nil {
		panic("rpc: call made when connection not started")
	}
	if conn.closing || conn.shutdown {
		call.Error = ErrShutdown
		conn.mutex.Unlock()
		call.done()
		return
	}
	conn.reqId++
	reqId := conn.reqId
	conn.clientPending[reqId] = call
	conn.mutex.Unlock()

	// Encode and send the request.
	hdr := &Header{
		RequestId: reqId,
		Request:   call.Request,
	}
	params := call.Params
	if params == nil {
		params = struct{}{}
	}
	if conn.notifier != nil {
		conn.notifier.ClientRequest(hdr, params)
	}
	if err := conn.codec.WriteMessage(hdr, params); err != nil {
		conn.mutex.Lock()
		call = conn.clientPending[reqId]
		delete(conn.clientPending, reqId)
		conn.mutex.Unlock()
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (conn *Conn) handleResponse(hdr *Header) error {
	reqId := hdr.RequestId
	conn.mutex.Lock()
	call := conn.clientPending[reqId]
	delete(conn.clientPending, reqId)
	conn.mutex.Unlock()

	var err error
	switch {
	case call == nil:
		// We've got no pending call. That usually means that
		// WriteHeader partially failed, and call was already
		// removed; response is a server telling us about an
		// error reading request body. We should still attempt
		// to read error body, but there's no one to give it to.
		if conn.notifier != nil {
			conn.notifier.ClientReply(Request{}, hdr, nil)
		}
		err = conn.readBody(nil, false)
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
		err = conn.readBody(nil, false)
		if conn.notifier != nil {
			conn.notifier.ClientReply(call.Request, hdr, nil)
		}
		call.done()
	default:
		err = conn.readBody(call.Response, false)
		if conn.notifier != nil {
			conn.notifier.ClientReply(call.Request, hdr, call.Response)
		}
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
func (conn *Conn) Call(req Request, params, response interface{}) error {
	call := &Call{
		Request:  req,
		Params:   params,
		Response: response,
		Done:     make(chan *Call, 1),
	}
	conn.send(call)
	result := <-call.Done
	return errors.Trace(result.Error)
}
