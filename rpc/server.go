// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"context"
	"io"
	"reflect"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/core/trace"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/rpcreflect"
)

const codeNotImplemented = "not implemented"

var logger = internallogger.GetLogger("juju.rpc")

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

	// Error holds the error string for a response. If there is no error,
	// this will be empty.
	Error string

	// ErrorCode holds the code of the error for a response. Error code will
	// be empty if there is no error.
	// TODO (stickupkid): This should be renamed to ResponseCode, that way
	// we're not confusing a programmatic error (empty string) with a
	// valid response code.
	ErrorCode string

	// ErrorInfo holds an optional set of additional information for an
	// error. This is used to provide additional information about the
	// error.
	// TODO (stickupkid): This should have been metadata for all responses
	// not just errors.
	ErrorInfo map[string]interface{}

	// Version defines the wire format of the request and response structure.
	Version int

	// TraceID holds the trace id of the request. This is used for sending
	// and receiving trace information.
	TraceID string

	// SpanID holds the span id of the request. This is used for sending
	// and receiving trace information.
	SpanID string

	// TraceFlags holds the trace flags of the request. This is used for
	// sending and receiving trace information.
	// Currently it indicates if a trace is sampled.
	TraceFlags int
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

// RecorderFactory is a function that returns a recorder to record
// details of a single request/response.
type RecorderFactory func() Recorder

// Recorder represents something the connection uses to record
// requests and replies. Recording a message can fail (for example for
// audit logging), and when it does the request should be failed as
// well.
type Recorder interface {
	HandleRequest(hdr *Header, body interface{}) error
	HandleReply(req Request, replyHdr *Header, body interface{}) error
}

type noopTracingRoot struct {
	rpcreflect.Value
}

func (noopTracingRoot) StartTrace(ctx context.Context) (context.Context, trace.Span) {
	return ctx, trace.NoopSpan{}
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

	// tombstones holds the client request ids that have been
	// cancelled.
	tombstones map[uint64]struct{}

	// closing is set when the connection is shutting down via
	// Close.  When this is set, no more client or server requests
	// will be initiated.
	closing bool

	// shutdown is set when the input loop terminates. When this
	// is set, no more client requests will be sent to the server.
	shutdown bool

	// dead is closed when the input loop terminates.
	dead chan struct{}

	// context is created when the connection is started, and is
	// cancelled when the connection is closed.
	context       context.Context
	cancelContext context.CancelFunc

	// inputLoopError holds the error that caused the input loop to
	// terminate prematurely.  It is set before dead is closed.
	inputLoopError error

	recorderFactory RecorderFactory
}

// NewConn creates a new connection that uses the given codec for
// transport, but it does not start it. Conn.Start must be called
// before any requests are sent or received. If recorderFactory is
// non-nil, it will be called to get a new recorder for every request.
func NewConn(codec Codec, factory RecorderFactory) *Conn {
	return &Conn{
		codec:           codec,
		clientPending:   make(map[uint64]*Call),
		tombstones:      make(map[uint64]struct{}),
		recorderFactory: ensureFactory(factory),
	}
}

// Start starts the RPC connection running.  It must be called at
// least once for any RPC connection (client or server side) It has no
// effect if it has already been called.  By default, a connection
// serves no methods.  See Conn.Serve for a description of how to
// serve methods on a Conn.
//
// The context passed in will be propagated to requests served by
// the connection.
func (conn *Conn) Start(ctx context.Context) {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	if conn.dead == nil {
		conn.context, conn.cancelContext = context.WithCancel(ctx)
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
//	M(id string) (O, error)
//
// where M is an exported name, conventionally naming the object type,
// id is some identifier for the object and O is the type of the
// returned object.
//
// Methods defined on O may defined in one of the following forms, where
// T and R must be struct types.
//
//	Method([context.Context])
//	Method([context.Context]) R
//	Method([context.Context]) (R, error)
//	Method([context.Context]) error
//	Method([context.Context,]T)
//	Method([context.Context,]T) R
//	Method([context.Context,]T) (R, error)
//	Method([context.Context,]T) error
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
func (conn *Conn) Serve(root interface{}, factory RecorderFactory, transformErrors func(error) error) {
	rootValue := rpcreflect.ValueOf(reflect.ValueOf(root))
	if rootValue.IsValid() {
		conn.serve(noopTracingRoot{
			Value: rootValue,
		}, factory, transformErrors)
	} else {
		conn.serve(nil, factory, transformErrors)
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
func (conn *Conn) ServeRoot(root Root, factory RecorderFactory, transformErrors func(error) error) {
	conn.serve(root, factory, transformErrors)
}

func (conn *Conn) serve(root Root, factory RecorderFactory, transformErrors func(error) error) {
	if transformErrors == nil {
		transformErrors = noopTransform
	}
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	conn.root = root
	conn.recorderFactory = ensureFactory(factory)
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
		// Kill calls down into the resources to stop all the resources which
		// includes watchers. The watches need to be killed in order for their
		// API methods to return, otherwise they are just waiting.
		conn.root.Kill()
	}
	conn.mutex.Unlock()

	// Wait for any outstanding server requests to complete
	// and write their replies before closing the codec. We
	// cancel the context so that any requests that would
	// block will be notified that the server is shutting
	// down.
	conn.cancelContext()
	conn.srvPending.Wait()

	conn.mutex.Lock()
	if conn.root != nil {
		// It is possible that since we last Killed the root, other resources
		// may have been added during some of the pending call resolutions.
		// So to release these resources, double tap the root.
		conn.root.Kill()
	}
	conn.mutex.Unlock()

	// Closing the codec should cause the input loop to terminate.
	if err := conn.codec.Close(); err != nil {
		logger.Debugf(context.TODO(), "error closing codec: %v", err)
	}
	<-conn.dead

	return conn.inputLoopError
}

// ErrorCoder represents any error that has an associated error code. An error
// code is a short string that represents the kind of an error.
type ErrorCoder interface {
	ErrorCode() string
}

// ErrorInfoProvider represents any error that can provide additional error
// information as a map.
type ErrorInfoProvider interface {
	ErrorInfo() map[string]interface{}
}

// Root represents a type that can be used to lookup a Method and place
// calls on that method.
type Root interface {
	Killer
	// FindMethod returns a MethodCaller for the given method name. The
	// method will be associated with the given facade and version.
	FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error)
	// StartTrace starts a trace for a given request.
	StartTrace(context.Context) (context.Context, trace.Span)
}

// Killer represents a type that can be asked to abort any outstanding
// requests.  The Kill method should return immediately.
type Killer interface {
	// Kill kills any outstanding requests.  It should return
	// immediately.
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

	if conn.closing || errors.Cause(err) == io.EOF {
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
	defer conn.cancelContext()
	for {
		var hdr Header
		err := conn.codec.ReadHeader(&hdr)
		switch {
		case errors.Cause(err) == io.EOF:
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

func (conn *Conn) getRecorder() Recorder {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	return conn.recorderFactory()
}

func (conn *Conn) handleRequest(hdr *Header) error {
	recorder := conn.getRecorder()
	req, err := conn.bindRequest(hdr)
	if err != nil {
		if err := recorder.HandleRequest(hdr, nil); err != nil {
			return errors.Trace(err)
		}
		if err := conn.readBody(nil, true); err != nil {
			return err
		}
		// We don't transform the error here. bindRequest will have
		// already transformed it and returned a zero req.
		return conn.writeErrorResponse(hdr, err, recorder)
	}
	var argp interface{}
	var arg reflect.Value
	if req.ParamsType() != nil {
		v := reflect.New(req.ParamsType())
		arg = v.Elem()
		argp = v.Interface()
	}
	if err := conn.readBody(argp, true); err != nil {
		if err := recorder.HandleRequest(hdr, nil); err != nil {
			return errors.Trace(err)
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
		return conn.writeErrorResponse(hdr, req.transformErrors(err), recorder)
	}
	var body interface{} = struct{}{}
	if req.ParamsType() != nil {
		body = arg.Interface()
	}
	if err := recorder.HandleRequest(hdr, body); err != nil {
		logger.Errorf(context.TODO(), "error recording request %+v with arg %+v: %T %+v", req, arg, err, err)
		return conn.writeErrorResponse(hdr, req.transformErrors(err), recorder)
	}
	conn.mutex.Lock()
	closing := conn.closing
	if !closing {
		conn.srvPending.Add(1)
		go conn.runRequest(req, arg, hdr.Version, recorder)
	}
	conn.mutex.Unlock()
	if closing {
		// We're closing down - no new requests may be initiated.
		return conn.writeErrorResponse(hdr, req.transformErrors(ErrShutdown), recorder)
	}
	return nil
}

func (conn *Conn) writeErrorResponse(reqHdr *Header, err error, recorder Recorder) error {
	conn.sending.Lock()
	defer conn.sending.Unlock()
	hdr := &Header{
		RequestId:  reqHdr.RequestId,
		Version:    reqHdr.Version,
		TraceID:    reqHdr.TraceID,
		SpanID:     reqHdr.SpanID,
		TraceFlags: reqHdr.TraceFlags,
	}
	if err, ok := err.(ErrorCoder); ok {
		hdr.ErrorCode = err.ErrorCode()
	} else {
		hdr.ErrorCode = ""
	}
	hdr.Error = err.Error()
	if err, ok := err.(ErrorInfoProvider); ok {
		hdr.ErrorInfo = err.ErrorInfo()
	}
	if err := recorder.HandleReply(reqHdr.Request, hdr, struct{}{}); err != nil {
		logger.Errorf(context.TODO(), "error recording reply %+v: %T %+v", hdr, err, err)
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
func (conn *Conn) runRequest(
	req boundRequest,
	arg reflect.Value,
	version int,
	recorder Recorder,
) {
	// If the request causes a panic, ensure we log that before closing the connection.
	defer func() {
		if panicResult := recover(); panicResult != nil {
			logger.Criticalf(conn.context,
				"panic running request %+v with arg %+v: %v\n%v", req, arg, panicResult, string(debug.Stack()))
			_ = conn.writeErrorResponse(&req.hdr, errors.Errorf("%v", panicResult), recorder)
		}
	}()
	defer conn.srvPending.Done()

	// Create a request-specific context, cancelled when the
	// request returns.
	ctx, cancel := context.WithCancel(conn.context)
	defer cancel()

	// If the request is a client request, then we need to
	// record the traceID and spanID from the request. If it's empty, we
	// don't care, a new one will be curated for us.
	ctx = trace.WithTraceScope(ctx, req.hdr.TraceID, req.hdr.SpanID, req.hdr.TraceFlags)

	conn.withTrace(ctx, req.hdr.Request, func(ctx context.Context) {
		conn.callRequest(ctx, req, arg, version, recorder)
	})
}

func (conn *Conn) withTrace(ctx context.Context, request Request, fn func(ctx context.Context)) {
	ctx, span := conn.root.StartTrace(ctx)
	defer span.End(
		trace.StringAttr("request.type", request.Type),
		trace.IntAttr("request.version", request.Version),
		trace.StringAttr("request.action", request.Action),
	)

	// Set the otel.traceid for the goroutine, so profiling tools can then link
	// the trace to the profile.
	traceID, _ := trace.TraceIDFromContext(ctx)
	pprof.Do(ctx, pprof.Labels(trace.OTELTraceID, traceID), func(ctx context.Context) {
		fn(ctx)
	})
}

func (conn *Conn) callRequest(
	ctx context.Context,
	req boundRequest,
	arg reflect.Value,
	version int,
	recorder Recorder,
) {
	rv, err := req.Call(ctx, req.hdr.Request.Id, arg)
	if err != nil {
		// Record the first error, this is the one that will be returned to
		// the client.
		trace.SpanFromContext(ctx).RecordError(err)
		err = conn.writeErrorResponse(&req.hdr, req.transformErrors(err), recorder)
	} else {
		hdr := &Header{
			RequestId:  req.hdr.RequestId,
			Version:    version,
			TraceID:    req.hdr.TraceID,
			SpanID:     req.hdr.SpanID,
			TraceFlags: req.hdr.TraceFlags,
		}
		var rvi interface{}
		if rv.IsValid() {
			rvi = rv.Interface()
		} else {
			rvi = struct{}{}
		}
		if err := recorder.HandleReply(req.hdr.Request, hdr, rvi); err != nil {
			logger.Errorf(context.TODO(), "error recording reply %+v: %T %+v", hdr, err, err)
		}
		conn.sending.Lock()
		err = conn.codec.WriteMessage(hdr, rvi)
		conn.sending.Unlock()
	}
	if err != nil {
		// If the message failed due to the other end closing the socket, that
		// is expected when an agent restarts so no need to log an  error.
		// The error type here is errors.errorString so all we can do is a match
		// on the error string content.
		msg := err.Error()
		if !strings.Contains(msg, "websocket: close sent") &&
			!strings.Contains(msg, "write: broken pipe") {

			// Record the second error, this is the one that will be recorded if
			// we can't write the response to the client.
			trace.SpanFromContext(ctx).RecordError(err)
			logger.Errorf(context.TODO(), "error writing response: %T %+v", err, err)
		}
	}
}

type serverError struct {
	error
}

func (e *serverError) ErrorCode() string {
	// serverError only knows one error code.
	return codeNotImplemented
}

func ensureFactory(f RecorderFactory) RecorderFactory {
	if f != nil {
		return f
	}
	var nop nopRecorder
	return func() Recorder {
		return &nop
	}
}

type nopRecorder struct{}

func (nopRecorder) HandleRequest(hdr *Header, body interface{}) error { return nil }

func (nopRecorder) HandleReply(req Request, hdr *Header, body interface{}) error { return nil }
