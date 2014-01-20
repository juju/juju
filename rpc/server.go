// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/rpc/rpcreflect"
)

const CodeNotImplemented = "not implemented"

var logger = loggo.GetLogger("juju.rpc")

// A Codec implements reading and writing of messages in an RPC
// session.  The RPC code calls WriteMessage to write a message to the
// connection and calls ReadHeader and ReadBody in pairs to read
// messages.
type Codec interface {
	// ReadHeader reads a message header into hdr.
	ReadHeader(hdr *Header) error

	// ReadBody reads a message body into the given body value.  The
	// isRequest parameter specifies whether the message being read
	// is a request; if not, it's a response.  The body value will
	// be a non-nil struct pointer, or nil to signify that the body
	// should be read and discarded.
	ReadBody(body interface{}, isRequest bool) error

	// WriteMessage writes a message with the given header and body.
	// The body will always be a struct. It may be called concurrently
	// with ReadHeader and ReadBody, but will not be called
	// concurrently with itself.
	WriteMessage(hdr *Header, body interface{}) error

	// Close closes the codec. It may be called concurrently
	// and should cause the Read methods to unblock.
	Close() error
}

// Header is a header written before every RPC call.  Since RPC requests
// can be initiated from either side, the header may represent a request
// from the other side or a response to an outstanding request.
type Header struct {
	// RequestId holds the sequence number of the request.
	// For replies, it holds the sequence number of the request
	// that is being replied to.
	RequestId uint64

	// Request holds the action to invoke.
	Request Request

	// Error holds the error, if any.
	Error string

	// ErrorCode holds the code of the error, if any.
	ErrorCode string
}

// Request represents an RPC to be performed, absent its parameters.
type Request struct {
	// Type holds the type of object to act on.
	Type string

	// Id holds the id of the object to act on.
	Id string

	// Action holds the action to perform on the object.
	Action string
}

// IsRequest returns whether the header represents an RPC request.  If
// it is not a request, it is a response.
func (hdr *Header) IsRequest() bool {
	return hdr.Request.Type != "" || hdr.Request.Action != ""
}

// Note that we use "client request" and "server request" to name
// requests initiated locally and remotely respectively.

// Conn represents an RPC endpoint.  It can both initiate and receive
// RPC requests.  There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by multiple goroutines
// simultaneously.
type Conn struct {
	// codec holds the underlying RPC connection.
	codec Codec

	// notifier is informed about RPC requests. It may be nil.
	notifier RequestNotifier

	// srvPending represents the current server requests.
	srvPending sync.WaitGroup

	// sending guards the write side of the codec - it ensures
	// that codec.WriteMessage is not called concurrently.
	// It also guards shutdown.
	sending sync.Mutex

	// mutex guards the following values.
	mutex sync.Mutex

	// rootValue holds the value to use to serve RPC requests, if any.
	rootValue rpcreflect.Value

	// transformErrors is used to transform returned errors.
	transformErrors func(error) error

	// reqId holds the latest client request id.
	reqId uint64

	// clientPending holds all pending client requests.
	clientPending map[uint64]*Call

	// closing is set when the connection is shutting down via
	// Close.  When this is set, no more client or server requests
	// will be initiated.
	closing bool

	// shutdown is set when the input loop terminates. When this
	// is set, no more client requests will be sent to the server.
	shutdown bool

	// dead is closed when the input loop terminates.
	dead chan struct{}

	// inputLoopError holds the error that caused the input loop to
	// terminate prematurely.  It is set before dead is closed.
	inputLoopError error
}

// RequestNotifier can be implemented to find out about requests
// occurring in an RPC conn, for example to print requests for logging
// purposes. The calls should not block or interact with the Conn object
// as that can cause delays to the RPC server or deadlock.
// Note that the methods on RequestNotifier may
// be called concurrently.
type RequestNotifier interface {
	// ServerRequest informs the RequestNotifier of a request made
	// to the Conn. If the request was not recognized or there was
	// an error reading the body, body will be nil.
	//
	// ServerRequest is called just before the server method
	// is invoked.
	ServerRequest(hdr *Header, body interface{})

	// ServerReply informs the RequestNotifier of a reply sent to a
	// server request. The given Request gives details of the call
	// that was made; the given Header and body are the header and
	// body sent as reply.
	//
	// ServerReply is called just before the reply is written.
	ServerReply(req Request, hdr *Header, body interface{}, timeSpent time.Duration)

	// ClientRequest informs the RequestNotifier of a request
	// made from the Conn. It is called just before the request is
	// written.
	ClientRequest(hdr *Header, body interface{})

	// ClientReply informs the RequestNotifier of a reply received
	// to a request. If the reply was to an unrecognised request,
	// the Request will be zero-valued. If the reply contained an
	// error, body will be nil; otherwise body will be the value that
	// was passed to the Conn.Call method.
	//
	// ClientReply is called just before the reply is handed to
	// back to the caller.
	ClientReply(req Request, hdr *Header, body interface{})
}

// NewConn creates a new connection that uses the given codec for
// transport, but it does not start it. Conn.Start must be called before
// any requests are sent or received. If notifier is non-nil, the
// appropriate method will be called for every RPC request.
func NewConn(codec Codec, notifier RequestNotifier) *Conn {
	return &Conn{
		codec:         codec,
		clientPending: make(map[uint64]*Call),
		notifier:      notifier,
	}
}

// Start starts the RPC connection running.  It must be called at least
// once for any RPC connection (client or server side) It has no effect
// if it has already been called.  By default, a connection serves no
// methods.  See Conn.Serve for a description of how to serve methods on
// a Conn.
func (conn *Conn) Start() {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	if conn.dead == nil {
		conn.dead = make(chan struct{})
		go conn.input()
	}
}

// Serve serves RPC requests on the connection by invoking methods on
// root. Note that it does not start the connection running,
// though it may be called once the connection is already started.
//
// The server executes each client request by calling a method on root
// to obtain an object to act on; then it invokes an method on that
// object with the request parameters, possibly returning some result.
//
// Methods on the root value are of the form:
//
//      M(id string) (O, error)
//
// where M is an exported name, conventionally naming the object type,
// id is some identifier for the object and O is the type of the
// returned object.
//
// Methods defined on O may defined in one of the following forms, where
// T and R must be struct types.
//
//	Method()
//	Method() R
//	Method() (R, error)
//	Method() error
//	Method(T)
//	Method(T) R
//	Method(T) (R, error)
//	Method(T) error
//
// If transformErrors is non-nil, it will be called on all returned
// non-nil errors, for example to transform the errors into ServerErrors
// with specified codes.  There will be a panic if transformErrors
// returns nil.
//
// Serve may be called at any time on a connection to change the
// set of methods being served by the connection. This will have
// no effect on calls that are currently being services.
// If root is nil, the connection will serve no methods.
func (conn *Conn) Serve(root interface{}, transformErrors func(error) error) {
	rootValue := rpcreflect.ValueOf(reflect.ValueOf(root))
	if rootValue.IsValid() && transformErrors == nil {
		transformErrors = func(err error) error { return err }
	}
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	conn.rootValue = rootValue
	conn.transformErrors = transformErrors
}

// Dead returns a channel that is closed when the connection
// has been closed or the underlying transport has received
// an error. There may still be outstanding requests.
// Dead must be called after conn.Start has been called.
func (conn *Conn) Dead() <-chan struct{} {
	return conn.dead
}

// Close closes the connection and its underlying codec; it returns when
// all requests have been terminated.
//
// If the connection is serving requests, and the root value implements
// the Killer interface, its Kill method will be called.  The codec will
// then be closed only when all its outstanding server calls have
// completed.
//
// Calling Close multiple times is not an error.
func (conn *Conn) Close() error {
	conn.mutex.Lock()
	if conn.closing {
		conn.mutex.Unlock()
		// Golang's net/rpc returns rpc.ErrShutdown if you ask to close
		// a closing or shutdown connection. Our choice is that Close
		// is an idempotent way to ask for resources to be released and
		// isn't a failure if called multiple times.
		return nil
	}
	conn.closing = true
	// Kill server requests if appropriate.  Client requests will be
	// terminated when the input loop finishes.
	if conn.rootValue.IsValid() {
		if killer, ok := conn.rootValue.GoValue().Interface().(Killer); ok {
			killer.Kill()
		}
	}
	conn.mutex.Unlock()

	// Wait for any outstanding server requests to complete
	// and write their replies before closing the codec.
	conn.srvPending.Wait()

	// Closing the codec should cause the input loop to terminate.
	if err := conn.codec.Close(); err != nil {
		logger.Infof("error closing codec: %v", err)
	}
	<-conn.dead
	return conn.inputLoopError
}

// ErrorCoder represents an any error that has an associated
// error code. An error code is a short string that represents the
// kind of an error.
type ErrorCoder interface {
	ErrorCode() string
}

// Killer represents a type that can be asked to abort any outstanding
// requests.  The Kill method should return immediately.
type Killer interface {
	Kill()
}

// input reads messages from the connection and handles them
// appropriately.
func (conn *Conn) input() {
	err := conn.loop()
	conn.sending.Lock()
	defer conn.sending.Unlock()
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if conn.closing || err == io.EOF {
		err = ErrShutdown
	} else {
		// Make the error available for Conn.Close to see.
		conn.inputLoopError = err
	}
	// Terminate all client requests.
	for _, call := range conn.clientPending {
		call.Error = err
		call.done()
	}
	conn.clientPending = nil
	conn.shutdown = true
	close(conn.dead)
}

// loop implements the looping part of Conn.input.
func (conn *Conn) loop() error {
	var hdr Header
	for {
		hdr = Header{}
		err := conn.codec.ReadHeader(&hdr)
		if err != nil {
			return err
		}
		if hdr.IsRequest() {
			err = conn.handleRequest(&hdr)
		} else {
			err = conn.handleResponse(&hdr)
		}
		if err != nil {
			return err
		}
	}
}

func (conn *Conn) readBody(resp interface{}, isRequest bool) error {
	if resp == nil {
		resp = &struct{}{}
	}
	return conn.codec.ReadBody(resp, isRequest)
}

func (conn *Conn) handleRequest(hdr *Header) error {
	startTime := time.Now()
	req, err := conn.bindRequest(hdr)
	if err != nil {
		if conn.notifier != nil {
			conn.notifier.ServerRequest(hdr, nil)
		}
		if err := conn.readBody(nil, true); err != nil {
			return err
		}
		// We don't transform the error because there
		// may be no transformErrors function available.
		return conn.writeErrorResponse(hdr, err, startTime)
	}
	var argp interface{}
	var arg reflect.Value
	if req.ParamsType != nil {
		v := reflect.New(req.ParamsType)
		arg = v.Elem()
		argp = v.Interface()
	}
	if err := conn.readBody(argp, true); err != nil {
		if conn.notifier != nil {
			conn.notifier.ServerRequest(hdr, nil)
		}
		// If we get EOF, we know the connection is a
		// goner, so don't try to respond.
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return err
		}
		// An error reading the body often indicates bad
		// request parameters rather than an issue with
		// the connection itself, so we reply with an
		// error rather than tearing down the connection
		// unless it's obviously a connection issue.  If
		// the error is actually a framing or syntax
		// problem, then the next ReadHeader should pick
		// up the problem and abort.
		return conn.writeErrorResponse(hdr, req.transformErrors(err), startTime)
	}
	if conn.notifier != nil {
		if req.ParamsType != nil {
			conn.notifier.ServerRequest(hdr, arg.Interface())
		} else {
			conn.notifier.ServerRequest(hdr, struct{}{})
		}
	}
	conn.mutex.Lock()
	closing := conn.closing
	if !closing {
		conn.srvPending.Add(1)
		go conn.runRequest(req, arg, startTime)
	}
	conn.mutex.Unlock()
	if closing {
		// We're closing down - no new requests may be initiated.
		return conn.writeErrorResponse(hdr, req.transformErrors(ErrShutdown), startTime)
	}
	return nil
}

func (conn *Conn) writeErrorResponse(reqHdr *Header, err error, startTime time.Time) error {
	conn.sending.Lock()
	defer conn.sending.Unlock()
	hdr := &Header{
		RequestId: reqHdr.RequestId,
	}
	if err, ok := err.(ErrorCoder); ok {
		hdr.ErrorCode = err.ErrorCode()
	} else {
		hdr.ErrorCode = ""
	}
	hdr.Error = err.Error()
	if conn.notifier != nil {
		conn.notifier.ServerReply(reqHdr.Request, hdr, struct{}{}, time.Since(startTime))
	}
	return conn.codec.WriteMessage(hdr, struct{}{})
}

// boundRequest represents an RPC request that is
// bound to an actual implementation.
type boundRequest struct {
	rpcreflect.MethodCaller
	transformErrors func(error) error
	hdr             Header
}

// bindRequest searches for methods implementing the
// request held in the given header and returns
// a boundRequest that can call those methods.
func (conn *Conn) bindRequest(hdr *Header) (boundRequest, error) {
	conn.mutex.Lock()
	rootValue := conn.rootValue
	transformErrors := conn.transformErrors
	conn.mutex.Unlock()

	if !rootValue.IsValid() {
		return boundRequest{}, fmt.Errorf("no service")
	}
	caller, err := rootValue.MethodCaller(hdr.Request.Type, hdr.Request.Action)
	if err != nil {
		if _, ok := err.(*rpcreflect.CallNotImplementedError); ok {
			err = &serverError{
				Message: err.Error(),
				Code:    CodeNotImplemented,
			}
		}
		return boundRequest{}, err
	}
	return boundRequest{
		MethodCaller:    caller,
		transformErrors: transformErrors,
		hdr:             *hdr,
	}, nil
}

// runRequest runs the given request and sends the reply.
func (conn *Conn) runRequest(req boundRequest, arg reflect.Value, startTime time.Time) {
	defer conn.srvPending.Done()
	rv, err := req.Call(req.hdr.Request.Id, arg)
	if err != nil {
		err = conn.writeErrorResponse(&req.hdr, req.transformErrors(err), startTime)
	} else {
		hdr := &Header{
			RequestId: req.hdr.RequestId,
		}
		var rvi interface{}
		if rv.IsValid() {
			rvi = rv.Interface()
		} else {
			rvi = struct{}{}
		}
		if conn.notifier != nil {
			conn.notifier.ServerReply(req.hdr.Request, hdr, rvi, time.Since(startTime))
		}
		conn.sending.Lock()
		err = conn.codec.WriteMessage(hdr, rvi)
		conn.sending.Unlock()
	}
	if err != nil {
		logger.Errorf("error writing response: %v", err)
	}
}

type serverError RequestError

func (e *serverError) Error() string {
	return e.Message
}

func (e *serverError) ErrorCode() string {
	return e.Code
}
