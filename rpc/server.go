// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"io"
	"reflect"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/rpc/rpcreflect"
)

const codeNotImplemented = "not implemented"

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

	// Version defines the wire format of the request and response structure.
	Version int
}

// Request represents an RPC to be performed, absent its parameters.
type Request struct {
	// Type holds the type of object to act on.
	Type string

	// Version holds the version of Type we will be acting on
	Version int

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

// ObserverFactory is a type which can construct a new Observer.
type ObserverFactory interface {

	// RPCObserver will return a new Observer usually constructed
	// from the state previously built up in the Observer. The
	// returned instance will be utilized per RPC request.
	RPCObserver() Observer
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

	// srvPending represents the current server requests.
	srvPending sync.WaitGroup

	// sending guards the write side of the codec - it ensures
	// that codec.WriteMessage is not called concurrently.
	// It also guards shutdown.
	sending sync.Mutex

	// mutex guards the following values.
	mutex sync.Mutex

	// root represents  the current root object that serves the RPC requests.
	// It may be nil if nothing is being served.
	root Root

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

	observerFactory ObserverFactory
}

// NewConn creates a new connection that uses the given codec for
// transport, but it does not start it. Conn.Start must be called before
// any requests are sent or received. If notifier is non-nil, the
// appropriate method will be called for every RPC request.
func NewConn(codec Codec, observerFactory ObserverFactory) *Conn {
	return &Conn{
		codec:           codec,
		clientPending:   make(map[uint64]*Call),
		observerFactory: observerFactory,
	}
}

// Start starts the RPC connection running.  It must be called at
// least once for any RPC connection (client or server side) It has no
// effect if it has already been called.  By default, a connection
// serves no methods.  See Conn.Serve for a description of how to
// serve methods on a Conn.
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
	if rootValue.IsValid() {
		conn.serve(rootValue, transformErrors)
	} else {
		conn.serve(nil, transformErrors)
	}
}

// ServeRoot is like Serve except that it gives the root object dynamic
// control over what methods are available instead of using reflection
// on the type.
//
// The server executes each client request by calling FindMethod to obtain a
// method to invoke. It invokes that method with the request parameters,
// possibly returning some result.
//
// The Kill method will be called when the connection is closed.
func (conn *Conn) ServeRoot(root Root, transformErrors func(error) error) {
	conn.serve(root, transformErrors)
}

func (conn *Conn) serve(root Root, transformErrors func(error) error) {
	if transformErrors == nil {
		transformErrors = noopTransform
	}
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	conn.root = root
	conn.transformErrors = transformErrors
}

// noopTransform is used when transformErrors is not supplied to Serve.
func noopTransform(err error) error {
	return err
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
	if conn.root != nil {
		conn.root.Kill()
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

// Root represents a type that can be used to lookup a Method and place
// calls on that method.
type Root interface {
	FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error)
	Killer
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
	for {
		var hdr Header
		err := conn.codec.ReadHeader(&hdr)
		switch {
		case err == io.EOF:
			// handle sentinel error specially
			return err
		case err != nil:
			return errors.Annotate(err, "codec.ReadHeader error")
		case hdr.IsRequest():
			if err := conn.handleRequest(&hdr); err != nil {
				return errors.Annotatef(err, "codec.handleRequest %#v error", hdr)
			}
		default:
			if err := conn.handleResponse(&hdr); err != nil {
				return errors.Annotatef(err, "codec.handleResponse %#v error", hdr)
			}
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
	observer := conn.observerFactory.RPCObserver()
	req, err := conn.bindRequest(hdr)
	if err != nil {
		observer.ServerRequest(hdr, nil)
		if err := conn.readBody(nil, true); err != nil {
			return err
		}
		// We don't transform the error here. bindRequest will have
		// already transformed it and returned a zero req.
		return conn.writeErrorResponse(hdr, err, observer)
	}
	var argp interface{}
	var arg reflect.Value
	if req.ParamsType() != nil {
		v := reflect.New(req.ParamsType())
		arg = v.Elem()
		argp = v.Interface()
	}
	if err := conn.readBody(argp, true); err != nil {
		observer.ServerRequest(hdr, nil)

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
		return conn.writeErrorResponse(hdr, req.transformErrors(err), observer)
	}
	if req.ParamsType() != nil {
		observer.ServerRequest(hdr, arg.Interface())
	} else {
		observer.ServerRequest(hdr, struct{}{})
	}
	conn.mutex.Lock()
	closing := conn.closing
	if !closing {
		conn.srvPending.Add(1)
		go conn.runRequest(req, arg, hdr.Version, observer)
	}
	conn.mutex.Unlock()
	if closing {
		// We're closing down - no new requests may be initiated.
		return conn.writeErrorResponse(hdr, req.transformErrors(ErrShutdown), observer)
	}
	return nil
}

func (conn *Conn) writeErrorResponse(reqHdr *Header, err error, observer Observer) error {
	conn.sending.Lock()
	defer conn.sending.Unlock()
	hdr := &Header{
		RequestId: reqHdr.RequestId,
		Version:   reqHdr.Version,
	}
	if err, ok := err.(ErrorCoder); ok {
		hdr.ErrorCode = err.ErrorCode()
	} else {
		hdr.ErrorCode = ""
	}
	hdr.Error = err.Error()
	observer.ServerReply(reqHdr.Request, hdr, struct{}{})

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
	root := conn.root
	transformErrors := conn.transformErrors
	conn.mutex.Unlock()

	if root == nil {
		return boundRequest{}, errors.New("no service")
	}
	caller, err := root.FindMethod(
		hdr.Request.Type, hdr.Request.Version, hdr.Request.Action)
	if err != nil {
		if _, ok := err.(*rpcreflect.CallNotImplementedError); ok {
			err = &serverError{
				error: err,
			}
		} else {
			err = transformErrors(err)
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
func (conn *Conn) runRequest(req boundRequest, arg reflect.Value, version int, observer Observer) {
	defer conn.srvPending.Done()
	rv, err := req.Call(req.hdr.Request.Id, arg)
	if err != nil {
		err = conn.writeErrorResponse(&req.hdr, req.transformErrors(err), observer)
	} else {
		hdr := &Header{
			RequestId: req.hdr.RequestId,
			Version:   version,
		}
		var rvi interface{}
		if rv.IsValid() {
			rvi = rv.Interface()
		} else {
			rvi = struct{}{}
		}
		observer.ServerReply(req.hdr.Request, hdr, rvi)
		conn.sending.Lock()
		err = conn.codec.WriteMessage(hdr, rvi)
		conn.sending.Unlock()
	}
	if err != nil {
		logger.Errorf("error writing response: %v", err)
	}
}

type serverError struct {
	error
}

func (e *serverError) ErrorCode() string {
	// serverError only knows one error code.
	return codeNotImplemented
}
