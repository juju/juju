// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/juju/errors"
)

// ErrShutdown is returned when a request is made on a connection that is
// shutting down.
var ErrShutdown = errors.New("connection is shut down")

// IsShutdownErr returns true if the error is ErrShutdown.
func IsShutdownErr(err error) bool {
	return errors.Cause(err) == ErrShutdown
}

// Call represents an active RPC.
type Call struct {
	Request
	Params     interface{}
	Response   interface{}
	Error      error
	Done       chan *Call
	TraceID    string
	SpanID     string
	TraceFlags int
}

// RequestError represents an error returned from an RPC request.
type RequestError struct {
	Message string
	Code    string
	Info    map[string]interface{}
}

func (e *RequestError) Error() string {
	if e.Code != "" {
		return e.Message + " (" + e.Code + ")"
	}
	return e.Message
}

// ErrorCode returns the error code associated with the error.
func (e *RequestError) ErrorCode() string {
	return e.Code
}

// ErrorInfo returns the error information associated with the error.
func (e *RequestError) ErrorInfo() map[string]interface{} {
	return e.Info
}

// UnmarshalInfo attempts to unmarshal the information contained in the Info
// field of a RequestError into an object instance a pointer to which is passed
// via the to argument. The method will return an error if a non-pointer arg
// is provided.
func (e *RequestError) UnmarshalInfo(to interface{}) error {
	if reflect.ValueOf(to).Kind() != reflect.Ptr {
		return errors.New("UnmarshalInfo expects a pointer as an argument")
	}

	data, err := json.Marshal(e.Info)
	if err != nil {
		return errors.Annotate(err, "could not marshal error information")
	}
	err = json.Unmarshal(data, to)
	if err != nil {
		return errors.Annotate(err, "could not unmarshal error information to provided target")
	}

	return nil
}

func (conn *Conn) send(call *Call) uint64 {
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
		return 0
	}
	conn.reqId++
	reqId := conn.reqId
	conn.clientPending[reqId] = call
	conn.mutex.Unlock()

	// Encode and send the request.
	hdr := &Header{
		RequestId:  reqId,
		Request:    call.Request,
		Version:    1,
		TraceID:    call.TraceID,
		SpanID:     call.SpanID,
		TraceFlags: call.TraceFlags,
	}
	params := call.Params
	if params == nil {
		params = struct{}{}
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

	return reqId
}

func (conn *Conn) cancel(reqID uint64) {
	conn.mutex.Lock()
	conn.tombstones[reqID] = struct{}{}
	delete(conn.clientPending, reqID)
	conn.mutex.Unlock()
}

func (conn *Conn) handleResponse(hdr *Header) error {
	reqId := hdr.RequestId
	conn.mutex.Lock()
	call := conn.clientPending[reqId]
	delete(conn.clientPending, reqId)
	conn.mutex.Unlock()

	defer func() {
		conn.mutex.Lock()
		// Always remove the tombstone after a call to prevent
		// unbounded growth.
		delete(conn.tombstones, reqId)
		conn.mutex.Unlock()
	}()

	var err error
	switch {
	case call == nil:
		// We've got no pending call. That usually means that
		// WriteHeader partially failed, and call was already
		// removed; response is a server telling us about an
		// error reading request body. We should still attempt
		// to read error body, but there's no one to give it to.
		err = conn.readBody(nil, false)

		// If the request has been canceled just return.
		if _, ok := conn.tombstones[reqId]; ok {
			return nil
		}
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
			Info:    hdr.ErrorInfo,
		}
		err = conn.readBody(nil, false)
		call.done()
	default:
		err = conn.readBody(call.Response, false)
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
func (conn *Conn) Call(ctx context.Context, req Request, params, response interface{}) error {
	// Before sending the request, check if the context has been canceled.
	// This is done to prevent any unnecessary work from being done if the
	// context has been canceled.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Extract the tracing information from the context.
	traceID, spanID, traceFlags := TracingFromContext(ctx)
	call := &Call{
		Request:    req,
		Params:     params,
		Response:   response,
		Done:       make(chan *Call, 1),
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: traceFlags,
	}
	reqID := conn.send(call)
	if reqID == 0 {
		// If the request ID is 0, the connection is shutting down or has
		// already shut down, then return the ErrShutdown error.
		return ErrShutdown
	}

	select {
	case <-ctx.Done():
		conn.cancel(reqID)
		return ctx.Err()
	case result := <-call.Done:
		return errors.Trace(result.Error)
	}
}
