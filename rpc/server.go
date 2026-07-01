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
	"sync/atomic"
	"time"

	"github.com/juju/juju/core/flightrecorder"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/rpcreflect"
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
	ReadBody(body any, isRequest bool) error

	// WriteMessage writes a message with the given header and body.
	// The body will always be a struct. It may be called concurrently
	// with ReadHeader, ReadBody, and other calls to WriteMessage.
	// Implementations must handle their own write serialization.
	WriteMessage(hdr *Header, body any) error

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
	ErrorInfo map[string]any

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
	HandleRequest(hdr *Header, body any) error
	HandleReply(req Request, replyHdr *Header, body any) error
}

type noopTracingRoot struct {
	rpcreflect.Value
}

func (noopTracingRoot) StartTrace(ctx context.Context) (context.Context, trace.Span) {
	return ctx, trace.NoopSpan{}
}

func (noopTracingRoot) FlightRecorder() flightrecorder.FlightRecorder {
	return flightrecorder.NoopRecorder{}
}

// Note that we use "client request" and "server request" to name
// requests initiated locally and remotely respectively.

// responseMsg represents a response message queued for writing.
type responseMsg struct {
	hdr  *Header
	body any
}

// serverConfig holds the server-side configuration that is set once
// (or rarely changed) via Serve/ServeRoot. It is stored in an atomic
// pointer so that the hot request path (bindRequest, getRecorder,
// withTrace) can read it without acquiring a mutex.
type serverConfig struct {
	root            Root
	transformErrors func(error) error
	recorderFactory RecorderFactory
}

// Conn represents an RPC endpoint.  It can both initiate and receive
// RPC requests.  There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by multiple goroutines
// simultaneously.
type Conn struct {
	// codec holds the underlying RPC connection.
	codec Codec

	// srvPending represents the current server requests.
	srvPending sync.WaitGroup

	// closing is set atomically when the connection is shutting down
	// via Close. When set, no more client or server requests will be
	// initiated. Using atomic.Bool allows the hot server request path
	// to check this without acquiring the mutex.
	closing atomic.Bool

	// config holds the server-side configuration (root, transformErrors,
	// recorderFactory). It is stored atomically and read lock-free on
	// the hot path.
	config atomic.Pointer[serverConfig]

	// reqId holds the latest client request id. It is accessed
	// atomically and does not require the mutex for correctness.
	reqId atomic.Uint64

	// mutex guards the client-side connection state: clientPending,
	// tombstones, shutdown, and dead. It also serializes Start()
	// initialization of context, dead, responses, and writerDone.
	mutex sync.Mutex

	// clientPending holds all pending client requests.
	clientPending map[uint64]*Call

	// tombstones holds the client request ids that have been
	// cancelled.
	tombstones map[uint64]struct{}

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
	// terminate prematurely. It is set before dead is closed, so
	// readers after <-dead observe it without a lock.
	inputLoopError error

	// responses is a buffered channel used to queue server response
	// messages for the writer goroutine. Handler goroutines push
	// responses here instead of writing directly to the codec,
	// eliminating head-of-line blocking between concurrent handlers.
	responses chan responseMsg

	// writerDone is closed when the writer goroutine exits.
	writerDone chan struct{}

	// pendingWrites tracks responses that have been queued but not yet
	// written by the writer goroutine. Close() waits on this to ensure
	// all responses are flushed before the codec is closed.
	pendingWrites sync.WaitGroup
}

// NewConn creates a new connection that uses the given codec for
// transport, but it does not start it. Conn.Start must be called
// before any requests are sent or received. If recorderFactory is
// non-nil, it will be called to get a new recorder for every request.
func NewConn(codec Codec, factory RecorderFactory) *Conn {
	conn := &Conn{
		codec:         codec,
		clientPending: make(map[uint64]*Call),
		tombstones:    make(map[uint64]struct{}),
	}
	conn.config.Store(&serverConfig{
		recorderFactory: ensureFactory(factory),
	})
	return conn
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
		conn.responses = make(chan responseMsg, 128)
		conn.writerDone = make(chan struct{})
		go conn.writer()
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
func (conn *Conn) Serve(root any, factory RecorderFactory, transformErrors func(error) error) {
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
	conn.config.Store(&serverConfig{
		root:            root,
		transformErrors: transformErrors,
		recorderFactory: ensureFactory(factory),
	})
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
	if !conn.closing.CompareAndSwap(false, true) {
		// Golang's net/rpc returns rpc.ErrShutdown if you ask to close
		// a closing or shutdown connection. Our choice is that Close
		// is an idempotent way to ask for resources to be released and
		// isn't a failure if called multiple times.
		return nil
	}
	if cfg := conn.config.Load(); cfg.root != nil {
		// Kill calls down into the resources to stop all the resources which
		// includes watchers. The watches need to be killed in order for their
		// API methods to return, otherwise they are just waiting.
		cfg.root.Kill()
	}

	// Wait for any outstanding server requests to complete
	// and write their replies before closing the codec. We
	// cancel the context so that any requests that would
	// block will be notified that the server is shutting
	// down.
	conn.cancelContext()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.srvPending.Wait()
	}()
	select {
	case <-done:
	case <-time.After(time.Minute):
		// A request is refusing to close, so we're blocked. We can't wait
		// indefinitely, and a minute is a lifetime for any request. Close it,
		// but warn in the logs that the connection is refusing to go away.
		logger.Warningf(conn.context, "timed out waiting for outstanding requests, closing anyway")
	}

	if cfg := conn.config.Load(); cfg.root != nil {
		// It is possible that since we last Killed the root, other resources
		// may have been added during some of the pending call resolutions.
		// So to release these resources, double tap the root.
		cfg.root.Kill()
	}

	// Wait for any responses queued by handler goroutines to be written
	// by the writer goroutine. This ensures all responses are flushed to
	// the client before the codec is closed.
	conn.pendingWrites.Wait()

	// Closing the codec should cause the input loop to terminate.
	if err := conn.codec.Close(); err != nil {
		logger.Debugf(conn.context, "error closing codec: %v", err)
	}
	<-conn.dead

	// Now that the input loop has exited and all handler goroutines are
	// done, close the responses channel to signal the writer to drain
	// any remaining messages and exit.
	close(conn.responses)
	<-conn.writerDone

	return conn.inputLoopError
}

// ErrorCoder represents any error that has an associated error code. An error
// code is a short string that represents the kind of an error.
type ErrorCoder interface {
	Error() string
	ErrorCode() string
}

// ErrorInfoProvider represents any error that can provide additional error
// information as a map.
type ErrorInfoProvider interface {
	Error() string
	ErrorInfo() map[string]any
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
	// FlightRecorder returns a flight recorder associated with the root.
	FlightRecorder() flightrecorder.FlightRecorder
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
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if conn.closing.Load() || errors.Is(err, io.EOF) {
		err = errors.Errorf(
			"connection is shut down: %w", err,
		).Add(ErrShutdown)
	} else {
		// Make the error available for Conn.Close to see.
		conn.inputLoopError = err
	}
	// Terminate all client requests.
	for _, call := range conn.clientPending {
		call.Error = err
		call.done(conn.context)
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
		case errors.Is(err, io.EOF):
			// handle sentinel error specially
			return err
		case err != nil:
			return errors.Errorf("codec.ReadHeader error: %w", err)
		case hdr.IsRequest():
			if err := conn.handleRequest(&hdr); err != nil {
				return errors.Errorf(
					"codec.handleRequest %#v error: %w", hdr, err,
				)
			}
		default:
			if err := conn.handleResponse(&hdr); err != nil {
				return errors.Errorf(
					"codec.handleResponse %#v error: %w", hdr, err,
				)
			}
		}
	}
}

func (conn *Conn) readBody(resp any, isRequest bool) error {
	if resp == nil {
		resp = &struct{}{}
	}
	return conn.codec.ReadBody(resp, isRequest)
}

func (conn *Conn) getRecorder() Recorder {
	return conn.config.Load().recorderFactory()
}

func (conn *Conn) handleRequest(hdr *Header) error {
	recorder := conn.getRecorder()
	req, err := conn.bindRequest(hdr)
	if err != nil {
		if err := recorder.HandleRequest(hdr, nil); err != nil {
			return errors.Capture(err)
		}
		if err := conn.readBody(nil, true); err != nil {
			return err
		}
		// We don't transform the error here. bindRequest will have
		// already transformed it and returned a zero req.
		conn.sendErrorResponse(hdr, err, recorder)
		return nil
	}
	var argp any
	var arg reflect.Value
	if req.ParamsType() != nil {
		v := reflect.New(req.ParamsType())
		arg = v.Elem()
		argp = v.Interface()
	}
	if err := conn.readBody(argp, true); err != nil {
		if err := recorder.HandleRequest(hdr, nil); err != nil {
			return errors.Capture(err)
		}

		// If we get EOF, we know the connection is a
		// goner, so don't try to respond.
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
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
		conn.sendErrorResponse(hdr, req.transformErrors(err), recorder)
		return nil
	}
	var body any = struct{}{}
	if req.ParamsType() != nil {
		body = arg.Interface()
	}
	if err := recorder.HandleRequest(hdr, body); err != nil {
		logger.Errorf(context.TODO(), "error recording request %+v with arg %+v: %T %+v", req, arg, err, err)
		conn.sendErrorResponse(hdr, req.transformErrors(err), recorder)
		return nil
	}

	if conn.closing.Load() {
		// We're closing down - no new requests may be initiated.
		conn.sendErrorResponse(hdr, req.transformErrors(ErrShutdown), recorder)
		return nil
	}

	conn.srvPending.Add(1)
	go conn.runRequest(req, arg, hdr.Version, recorder)

	return nil
}

// sendErrorResponse constructs an error response and queues it for writing.
// It calls recorder.HandleReply before queuing, ensuring observer work is not
// performed while holding any write lock.
func (conn *Conn) sendErrorResponse(reqHdr *Header, err error, recorder Recorder) {
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
	conn.sendResponse(hdr, struct{}{})
}

// sendResponse queues a response message for the writer goroutine.
// It blocks if the response buffer is full, providing natural backpressure.
// If the connection is dead, the response is dropped.
func (conn *Conn) sendResponse(hdr *Header, body any) {
	conn.pendingWrites.Add(1)
	select {
	case conn.responses <- responseMsg{hdr: hdr, body: body}:
	case <-conn.dead:
		conn.pendingWrites.Done()
	}
}

// writer is the dedicated goroutine that drains the responses channel
// and writes messages to the codec sequentially. This decouples handler
// goroutines from write latency and provides natural backpressure via
// the channel buffer.
func (conn *Conn) writer() {
	defer close(conn.writerDone)
	for msg := range conn.responses {
		err := conn.codec.WriteMessage(msg.hdr, msg.body)
		conn.pendingWrites.Done()
		if err != nil {
			msg := err.Error()
			if !strings.Contains(msg, "websocket: close sent") &&
				!strings.Contains(msg, "write: broken pipe") {
				logger.Errorf(conn.context, "error writing response: %T %+v", err, err)
			}
		}
	}
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
	cfg := conn.config.Load()

	if cfg.root == nil {
		return boundRequest{}, errors.New("no service")
	}
	caller, err := cfg.root.FindMethod(
		hdr.Request.Type, hdr.Request.Version, hdr.Request.Action)
	if err != nil {
		if _, ok := err.(*rpcreflect.CallNotImplementedError); ok {
			err = &serverError{
				error: err,
			}
		} else {
			err = cfg.transformErrors(err)
		}
		return boundRequest{}, err
	}
	return boundRequest{
		MethodCaller:    caller,
		transformErrors: cfg.transformErrors,
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
	// Close the service pending last, so that if there is a panic in the
	// request handling, we don't mark the request as done before writing the
	// error response.
	defer conn.srvPending.Done()

	// If the request causes a panic, ensure we log that before closing the
	// connection.
	defer func() {
		if panicResult := recover(); panicResult != nil {
			logger.Criticalf(conn.context,
				"panic running request %+v with arg %+v: %v\n%v", req, arg, panicResult, string(debug.Stack()))
			conn.sendErrorResponse(&req.hdr, errors.Errorf("%v", panicResult), recorder)
		}
	}()

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
	cfg := conn.config.Load()
	if cfg.root == nil {
		fn(ctx)
		return
	}

	ctx, span := cfg.root.StartTrace(ctx)
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
		if err := conn.getFlightRecorder().Capture(flightrecorder.KindError); err != nil {
			logger.Tracef(ctx, "error capturing flight recorder: %v", err)
		}

		// Record the first error, this is the one that will be returned to
		// the client.
		trace.SpanFromContext(ctx).RecordError(err)
		conn.sendErrorResponse(&req.hdr, req.transformErrors(err), recorder)
	} else {
		hdr := &Header{
			RequestId:  req.hdr.RequestId,
			Version:    version,
			TraceID:    req.hdr.TraceID,
			SpanID:     req.hdr.SpanID,
			TraceFlags: req.hdr.TraceFlags,
		}
		var rvi any
		if rv.IsValid() {
			rvi = rv.Interface()
		} else {
			rvi = struct{}{}
		}
		if err := recorder.HandleReply(req.hdr.Request, hdr, rvi); err != nil {
			logger.Errorf(ctx, "error recording reply %+v: %T %+v", hdr, err, err)
		}

		if err := conn.getFlightRecorder().Capture(flightrecorder.KindRequest); err != nil {
			logger.Tracef(ctx, "error capturing flight recorder: %v", err)
		}

		conn.sendResponse(hdr, rvi)
	}
}

var noop = flightrecorder.NoopRecorder{}

func (conn *Conn) getFlightRecorder() flightrecorder.FlightRecorder {
	if cfg := conn.config.Load(); cfg.root != nil {
		return cfg.root.FlightRecorder()
	}
	return noop
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

func (nopRecorder) HandleRequest(hdr *Header, body any) error { return nil }

func (nopRecorder) HandleReply(req Request, hdr *Header, body any) error { return nil }
