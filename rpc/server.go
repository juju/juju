// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"errors"
	"fmt"
	"io"
	"launchpad.net/juju-core/log"
	"launchpad.net/tomb"
	"reflect"
	"sync"
)

// A Codec implements reading and writing of RPC messages
// in an RPC session. The RPC code calls WriteMessage to
// write a message to the connection and calls
// ReadHeader and ReadBody in pairs to read messages.
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
	// The body will always be a struct.
	WriteMessage(hdr *Header, body interface{}) error

	// Close closes the codec. It may be called concurrently
	// and should cause the Read methods to unblock.
	Close() error
}

// Header is a header written before every RPC call.
// Since RPC requests can be initiated from either side,
// the header may represent a request from the other
// side or a response to an outstanding request.
type Header struct {
	// RequestId holds the sequence number of the request.
	RequestId uint64

	// Type holds the type of object to act on.
	Type string

	// Id holds the id of the object to act on.
	Id string

	// Request holds the action to invoke on the remote object.
	Request string

	// Error holds the error, if any.
	Error string

	// ErrorCode holds the code of the error, if any.
	ErrorCode string
}

// IsRequest returns whether the header represents
// an RPC request. If it is not a request, it is a response.
func (hdr *Header) IsRequest() bool {
	return hdr.Type != "" || hdr.Request != ""
}

// Note that we use "client request" and "server request" to name requests
// initiated locally and remotely respectively.

// Conn represents an RPC endpoint. It can both initiate
// and receive RPC requests. There may be multiple outstanding
// Calls associated with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
type Conn struct {
	// codec holds the underlying RPC connection.
	codec Codec

	// tomb is used to wait for the connection
	// to terminate in an orderly fashion.
	tomb tomb.Tomb

	// srvPending represents the currently server requests.
	srvPending sync.WaitGroup

	// sending guards the write side of the codec,
	// also including request below.
	sending sync.Mutex

	// srcMutex guards rootValue and transformErrors.
	srvMutex sync.Mutex

	// rootValue holds the value to use to serve RPC requests, if any.
	rootValue reflect.Value

	// transformErrors is used to transform returned errors.
	transformErrors func(error) error

	// clientMutex protects the following fields.
	clientMutex sync.Mutex

	// reqId holds the latest client request id.
	reqId uint64

	// clientPending holds all pending client requests.
	clientPending map[uint64]*Call

	closing  bool
	shutdown bool
}

func newConn(codec Codec) *Conn {
	return &Conn{
		codec:         codec,
		clientPending: make(map[uint64]*Call),
	}
}

// NewClient returns an RPC endpoint that uses the given codec for
// transport.  It does not serve any methods (the Serve method can be
// called later if desired). Other than the fact that NewClient does not
// serve requests, there is no necessary assocation of the Conn
// with the client side of a connection - both ends may well
// call NewServer if they wish.
func NewClient(codec Codec) *Conn {
	conn := newConn(codec)
	go conn.input()
	return conn
}

// NewServer returns an RPC endpoint that uses the given codec
// for transport. It serves the methods from the given value,
// transforming any returned error values with transformErrors.
// See Conn.Serve for a description of how RPC requests
// cause methods to be called,
func NewServer(codec Codec, rootValue interface{}, transformErrors func(error) error) (*Conn, error) {
	conn := newConn(codec)
	if err := conn.Serve(rootValue, transformErrors); err != nil {
		return nil, err
	}
	go conn.input()
	return conn, nil
}

// Serve serves RPC requests on the connection by invoking
// methods on rootValue.
//
// The server executes each client request by calling a method on
// root to obtain an object to act on; then it invokes an
// method on that object with the request parameters, possibly
// returning some result.
//
// Methods on the root value are of the form:
//
//      M(id string) (O, error)
//
// where M is an exported name, conventionally naming the object type,
// id is some identifier for the object and O is the type of the
// returned object.
//
// Methods defined on O may defined in one of the following
// forms, where T and R must be struct types.
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
// It is an error if the connection is already serving requests.
//
// If an error is encountered reading the connection and root implements
// the Killer interface, its Kill method will be called.
// The connection will then terminate only when all its outstanding calls have
// completed.
func (conn *Conn) Serve(root interface{}, transformErrors func(error) error) error {
	if transformErrors == nil {
		transformErrors = func(err error) error { return err }
	}
	rootValue := reflect.ValueOf(root)
	// Check that rootValue is ok to use as an RPC server type.
	_, err := methods(rootValue.Type())
	if err != nil {
		return err
	}
	conn.srvMutex.Lock()
	defer conn.srvMutex.Unlock()
	if conn.transformErrors != nil {
		return errors.New("RPC connection is already serving requests")
	}
	conn.rootValue = rootValue
	conn.transformErrors = transformErrors
	return nil
}

// Dead returns a channel that is closed when the connection completes.
func (conn *Conn) Dead() <-chan struct{} {
	return conn.tomb.Dead()
}

// Wait waits until the rpc connection has been dropped or closed
// and all client and server requests have terminated.
// The underlying codec will be closed.
func (conn *Conn) Wait() error {
	return conn.tomb.Wait()
}

// Close closes the connection and its underlying codec.  It does not
// wait for requests to terminate.
func (conn *Conn) Close() error {
	conn.clientMutex.Lock()
	conn.closing = true
	conn.clientMutex.Unlock()
	return conn.codec.Close()
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
	log.Infof("Conn.loop finished with err %v", err)
	conn.terminateClientRequests(err)
	conn.srvMutex.Lock()
	if conn.rootValue.IsValid() {
		log.Infof("killing rootValue")
		if killer, ok := conn.rootValue.Interface().(Killer); ok {
			killer.Kill()
		}
	}
	conn.srvMutex.Unlock()
	conn.tomb.Kill(err)
	conn.srvPending.Wait()
	conn.tomb.Done()
}

// loop implements the bulk of Conn.input.
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
	panic("unreachable")
}

// terminateClientRequests terminates any outstanding RPC calls, causing them to
// return an error.  The read error that caused the connection to
// terminate is provided in readErr.
func (conn *Conn) terminateClientRequests(readErr error) {
	conn.sending.Lock()
	defer conn.sending.Unlock()
	conn.clientMutex.Lock()
	defer conn.clientMutex.Unlock()
	conn.shutdown = true
	closing := conn.closing
	if readErr == io.EOF {
		if closing {
			readErr = ErrShutdown
		} else {
			readErr = io.ErrUnexpectedEOF
		}
	}
	for _, call := range conn.clientPending {
		call.Error = readErr
		call.done()
	}
	conn.clientPending = nil
	if readErr != io.EOF && !closing {
		log.Errorf("rpc: protocol error: %v", readErr)
	}
}

func (conn *Conn) readBody(resp interface{}, isRequest bool) error {
	if resp == nil {
		resp = &struct{}{}
	}
	return conn.codec.ReadBody(resp, isRequest)
}

func (conn *Conn) handleRequest(hdr *Header) error {
	o, a, err := conn.findRequest(hdr)
	if err != nil {
		if err := conn.readBody(nil, true); err != nil {
			return err
		}
		return conn.writeErrorResponse(hdr.RequestId, err)
	}
	var argp interface{}
	var arg reflect.Value
	if a.arg != nil {
		v := reflect.New(a.arg)
		arg = v.Elem()
		argp = v.Interface()
	}
	if err := conn.readBody(argp, true); err != nil {
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
		return conn.writeErrorResponse(hdr.RequestId, err)
	}
	conn.srvPending.Add(1)
	go conn.runRequest(hdr.RequestId, hdr.Id, o, a, arg)
	return nil
}

func (conn *Conn) writeErrorResponse(reqId uint64, err error) error {
	conn.sending.Lock()
	defer conn.sending.Unlock()
	hdr := &Header{
		RequestId: reqId,
	}
	err = conn.transformErrors(err)
	if err, ok := err.(ErrorCoder); ok {
		hdr.ErrorCode = err.ErrorCode()
	} else {
		hdr.ErrorCode = ""
	}
	hdr.Error = err.Error()
	if err := conn.codec.WriteMessage(hdr, struct{}{}); err != nil {
		return err
	}
	return nil
}

func (conn *Conn) isServing() bool {
	conn.srvMutex.Lock()
	defer conn.srvMutex.Unlock()
	return conn.transformErrors != nil
}

func (conn *Conn) findRequest(hdr *Header) (*obtainer, *action, error) {
	if !conn.isServing() {
		return nil, nil, fmt.Errorf("no service")
	}
	m, err := methods(conn.rootValue.Type())
	if err != nil {
		panic("failed to get methods")
	}
	o := m.obtain[hdr.Type]
	if o == nil {
		return nil, nil, fmt.Errorf("unknown object type %q", hdr.Type)
	}
	a := m.action[o.ret][hdr.Request]
	if a == nil {
		return nil, nil, fmt.Errorf("no such request %q on %s", hdr.Request, hdr.Type)
	}
	return o, a, nil
}

// runRequest runs the given request and sends the reply.
func (conn *Conn) runRequest(reqId uint64, objId string, o *obtainer, a *action, arg reflect.Value) {
	defer conn.srvPending.Done()
	rv, err := conn.runRequest0(reqId, objId, o, a, arg)
	if err != nil {
		err = conn.writeErrorResponse(reqId, err)
	} else {
		var rvi interface{}
		hdr := &Header{
			RequestId: reqId,
		}
		conn.sending.Lock()
		defer conn.sending.Unlock()
		if rv.IsValid() {
			rvi = rv.Interface()
		} else {
			rvi = struct{}{}
		}
		err = conn.codec.WriteMessage(hdr, rvi)
	}
	if err != nil {
		log.Errorf("rpc: error writing response: %v", err)
	}
}

func (conn *Conn) runRequest0(reqId uint64, objId string, o *obtainer, a *action, arg reflect.Value) (reflect.Value, error) {
	obj, err := o.call(conn.rootValue, objId)
	if err != nil {
		return reflect.Value{}, err
	}
	return a.call(obj, arg)
}
