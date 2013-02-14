package rpc_test

import (
	"encoding/json"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/testing"
	"net"
	"reflect"
	"sync"
	stdtesting "testing"
	"time"
)

type suite struct {
	testing.LoggingSuite
}

var _ = Suite(&suite{})

func TestAll(t *stdtesting.T) {
	TestingT(t)
}

type callInfo struct {
	rcvr   interface{}
	method string
	arg    interface{}
}

type callError callInfo

func (e *callError) Error() string {
	return fmt.Sprintf("error calling %s", e.method)
}

type stringVal struct {
	Val string
}

type TRoot struct {
	mu        sync.Mutex
	calls     []*callInfo
	returnErr bool
	simple    map[string]*SimpleMethods
	delayed   map[string]*DelayedMethods
	errorInst *ErrorMethods
}

func (r *TRoot) callError(rcvr interface{}, name string, arg interface{}) error {
	if r.returnErr {
		return &callError{rcvr, name, arg}
	}
	return nil
}

func (r *TRoot) SimpleMethods(id string) (*SimpleMethods, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.simple[id]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("unknown SimpleMethods id")
}

func (r *TRoot) DelayedMethods(id string) (*DelayedMethods, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.delayed[id]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("unknown DelayedMethods id")
}

func (r *TRoot) ErrorMethods(id string) (*ErrorMethods, error) {
	return r.errorInst, nil
}

func (t *TRoot) called(rcvr interface{}, method string, arg interface{}) {
	t.mu.Lock()
	t.calls = append(t.calls, &callInfo{rcvr, method, arg})
	t.mu.Unlock()
}

type SimpleMethods struct {
	root *TRoot
	id   string
}

// Each Call method is named in this standard form:
//
//     Call<narg>r<nret><e>
//
// where narg is the number of arguments, nret is the number of returned
// values (not including the error) and e is the letter 'e' if the
// method returns an error.

func (a *SimpleMethods) Call0r0() {
	a.root.called(a, "Call0r0", nil)
}

func (a *SimpleMethods) Call0r1() stringVal {
	a.root.called(a, "Call0r1", nil)
	return stringVal{"Call0r1 ret"}
}

func (a *SimpleMethods) Call0r1e() (stringVal, error) {
	a.root.called(a, "Call0r1e", nil)
	return stringVal{"Call0r1e ret"}, a.root.callError(a, "Call0r1e", nil)
}

func (a *SimpleMethods) Call0r0e() error {
	a.root.called(a, "Call0r0e", nil)
	return a.root.callError(a, "Call0r0e", nil)
}

func (a *SimpleMethods) Call1r0(s stringVal) {
	a.root.called(a, "Call1r0", s)
}

func (a *SimpleMethods) Call1r1(s stringVal) stringVal {
	a.root.called(a, "Call1r1", s)
	return stringVal{"Call1r1 ret"}
}

func (a *SimpleMethods) Call1r1e(s stringVal) (stringVal, error) {
	a.root.called(a, "Call1r1e", s)
	return stringVal{"Call1r1e ret"}, a.root.callError(a, "Call1r1e", s)
}

func (a *SimpleMethods) Call1r0e(s stringVal) error {
	a.root.called(a, "Call1r0e", s)
	return a.root.callError(a, "Call1r0e", s)
}

type DelayedMethods struct {
	ready chan struct{}
	done  chan string
}

func (a *DelayedMethods) Delay() stringVal {
	if a.ready != nil {
		a.ready <- struct{}{}
	}
	return stringVal{<-a.done}
}

type ErrorMethods struct {
	err error
}

func (e *ErrorMethods) Call() error {
	return e.err
}

func (*suite) TestRPC(c *C) {
	root := &TRoot{
		simple: make(map[string]*SimpleMethods),
	}
	root.simple["a99"] = &SimpleMethods{root: root, id: "a99"}
	client, srvDone := newRPCClientServer(c, root)
	for narg := 0; narg < 2; narg++ {
		for nret := 0; nret < 2; nret++ {
			for nerr := 0; nerr < 2; nerr++ {
				retErr := nerr != 0
				root.testCall(c, client, narg, nret, retErr, false)
				if retErr {
					root.testCall(c, client, narg, nret, retErr, true)
				}
			}
		}
	}
	client.Close()
	err := chanReadError(c, srvDone, "server done")
	c.Assert(err, IsNil)
}

func (root *TRoot) testCall(c *C, client *rpc.Client, narg, nret int, retErr, testErr bool) {
	root.calls = nil
	root.returnErr = testErr
	e := ""
	if retErr {
		e = "e"
	}
	method := fmt.Sprintf("Call%dr%d%s", narg, nret, e)
	c.Logf("test call %s", method)
	var r stringVal
	err := client.Call("SimpleMethods", "a99", method, stringVal{"arg"}, &r)
	root.mu.Lock()
	defer root.mu.Unlock()
	expectCall := callInfo{
		rcvr:   root.simple["a99"],
		method: method,
	}
	if narg > 0 {
		expectCall.arg = stringVal{"arg"}
	}
	c.Assert(root.calls, HasLen, 1)
	c.Assert(*root.calls[0], Equals, expectCall)
	switch {
	case retErr && testErr:
		c.Assert(err, DeepEquals, &rpc.ServerError{
			Message: fmt.Sprintf("error calling %s", method),
		})
		c.Assert(r, Equals, stringVal{})
	case nret > 0:
		c.Assert(r, Equals, stringVal{method + " ret"})
	}
}

func (*suite) TestConcurrentCalls(c *C) {
	start1 := make(chan string)
	start2 := make(chan string)
	ready1 := make(chan struct{})
	ready2 := make(chan struct{})

	root := &TRoot{
		delayed: map[string]*DelayedMethods{
			"1": {ready: ready1, done: start1},
			"2": {ready: ready2, done: start2},
		},
	}

	client, srvDone := newRPCClientServer(c, root)
	call := func(id string, done chan<- struct{}) {
		var r stringVal
		err := client.Call("DelayedMethods", id, "Delay", nil, &r)
		c.Check(err, IsNil)
		c.Check(r.Val, Equals, "return "+id)
		done <- struct{}{}
	}
	done1 := make(chan struct{})
	done2 := make(chan struct{})
	go call("1", done1)
	go call("2", done2)

	// Check that both calls are running concurrently.
	chanRead(c, ready1, "method 1 ready")
	chanRead(c, ready2, "method 2 ready")

	// Let the requests complete.
	start1 <- "return 1"
	start2 <- "return 2"
	chanRead(c, done1, "method 1 done")
	chanRead(c, done2, "method 2 done")
	client.Close()
	err := chanReadError(c, srvDone, "server done")
	c.Assert(err, IsNil)
}

type codedError struct {
	m    string
	code string
}

func (e *codedError) Error() string {
	return e.m
}

func (e *codedError) ErrorCode() string {
	return e.code
}

func (*suite) TestErrorCode(c *C) {
	root := &TRoot{
		errorInst: &ErrorMethods{&codedError{"message", "code"}},
	}
	client, srvDone := newRPCClientServer(c, root)
	err := client.Call("ErrorMethods", "", "Call", nil, nil)
	c.Assert(err, ErrorMatches, `server error: message \(code\)`)
	c.Assert(err.(rpc.ErrorCoder).ErrorCode(), Equals, "code")
	client.Close()
	err = chanReadError(c, srvDone, "server done")
	c.Assert(err, IsNil)
}

func (*suite) TestServerWaitsForOutstandingCalls(c *C) {
	ready := make(chan struct{})
	start := make(chan string)
	root := &TRoot{
		delayed: map[string]*DelayedMethods{
			"1": {
				ready: ready,
				done:  start,
			},
		},
	}
	client, srvDone := newRPCClientServer(c, root)
	done := make(chan struct{})
	go func() {
		var r stringVal
		err := client.Call("DelayedMethods", "1", "Delay", nil, &r)
		c.Check(err, FitsTypeOf, &net.OpError{})
		done <- struct{}{}
	}()
	chanRead(c, ready, "DelayedMethods.Delay ready")
	client.Close()
	select {
	case err := <-srvDone:
		c.Fatalf("server returned while outstanding operation in progress: %v", err)
		<-done
	case <-time.After(25 * time.Millisecond):
	}
	start <- "xxx"
	err := chanReadError(c, srvDone, "server done")
	c.Check(err, IsNil)
	chanRead(c, done, "DelayedMethods.Delay done")
}

func chanRead(c *C, ch <-chan struct{}, what string) {
	select {
	case <-ch:
		return
	case <-time.After(3 * time.Second):
		c.Fatalf("timeout on channel read %s", what)
	}
}

func (*suite) TestCompatibility(c *C) {
	root := &TRoot{
		simple: make(map[string]*SimpleMethods),
	}
	a0 := &SimpleMethods{root: root, id: "a0"}
	root.simple["a0"] = a0

	client, srvDone := newRPCClientServer(c, root)
	call := func(method string, arg, ret interface{}) (passedArg interface{}) {
		root.calls = nil
		err := client.Call("SimpleMethods", "a0", method, arg, ret)
		c.Assert(err, IsNil)
		c.Assert(root.calls, HasLen, 1)
		info := root.calls[0]
		c.Assert(info.rcvr, Equals, a0)
		c.Assert(info.method, Equals, method)
		return info.arg
	}
	type extra struct {
		Val   string
		Extra string
	}
	// Extra fields in request and response.
	var r extra
	arg := call("Call1r1", extra{"x", "y"}, &r)
	c.Assert(arg, Equals, stringVal{"x"})

	// Nil argument as request.
	r = extra{}
	arg = call("Call1r1", nil, &r)
	c.Assert(arg, Equals, stringVal{})

	// Nil argument as response.
	arg = call("Call1r1", stringVal{"x"}, nil)
	c.Assert(arg, Equals, stringVal{"x"})

	// Non-nil argument for no response.
	r = extra{}
	arg = call("Call1r0", stringVal{"x"}, &r)
	c.Assert(arg, Equals, stringVal{"x"})
	c.Assert(r, Equals, extra{})

	client.Close()
	err := chanReadError(c, srvDone, "server done")
	c.Assert(err, IsNil)
}

func (*suite) TestBadCall(c *C) {
	root := &TRoot{
		simple: make(map[string]*SimpleMethods),
	}
	a0 := &SimpleMethods{root: root, id: "a0"}
	root.simple["a0"] = a0
	client, srvDone := newRPCClientServer(c, root)

	err := client.Call("BadSomething", "a0", "No", nil, nil)
	c.Assert(err, ErrorMatches, `server error: unknown object type "BadSomething"`)

	err = client.Call("SimpleMethods", "xx", "No", nil, nil)
	c.Assert(err, ErrorMatches, `server error: no such request "No" on SimpleMethods`)

	err = client.Call("SimpleMethods", "xx", "Call0r0", nil, nil)
	c.Assert(err, ErrorMatches, "server error: unknown SimpleMethods id")

	client.Close()
	err = chanReadError(c, srvDone, "server done")
	c.Assert(err, IsNil)
}

func chanReadError(c *C, ch <-chan error, what string) error {
	select {
	case e := <-ch:
		return e
	case <-time.After(3 * time.Second):
		c.Fatalf("timeout on channel read %s", what)
	}
	panic("unreachable")
}

// newRPCClientServer starts an RPC server serving a connection from a
// single client.  When the server has finished serving the connection,
// it sends a value on done.
func newRPCClientServer(c *C, root interface{}) (client *rpc.Client, done <-chan error) {
	srv, err := rpc.NewServer(&TRoot{})
	c.Assert(err, IsNil)

	l, err := net.Listen("tcp", ":0")
	c.Assert(err, IsNil)
	defer l.Close()

	srvDone := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			srvDone <- err
			return
		}
		err = srv.ServeCodec(NewJSONServerCodec(conn), root)
		c.Logf("server status: %v", err)
		srvDone <- err
	}()
	conn, err := net.Dial("tcp", l.Addr().String())
	c.Assert(err, IsNil)
	client = rpc.NewClientWithCodec(NewJSONClientCodec(conn))
	return client, srvDone
}

type generalServerCodec struct {
	io.Closer
	enc encoder
	dec decoder
}

type encoder interface {
	Encode(e interface{}) error
}

type decoder interface {
	Decode(e interface{}) error
}

func (c *generalServerCodec) ReadRequestHeader(req *rpc.Request) error {
	return c.dec.Decode(req)
}

func (c *generalServerCodec) ReadRequestBody(argp interface{}) error {
	if v := reflect.ValueOf(argp); v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		panic(fmt.Errorf("ReadRequestBody bad destination; want *struct got %T", argp))
	}
	return c.dec.Decode(argp)
}

func (c *generalServerCodec) WriteResponse(resp *rpc.Response, x interface{}) error {
	if reflect.ValueOf(x).Kind() != reflect.Struct {
		panic(fmt.Errorf("WriteResponse bad result; want struct got %T (%#v)", x, x))
	}
	if err := c.enc.Encode(resp); err != nil {
		return err
	}
	return c.enc.Encode(x)
}

type generalClientCodec struct {
	io.Closer
	enc encoder
	dec decoder
}

func (c *generalClientCodec) WriteRequest(req *rpc.Request, x interface{}) error {
	if reflect.ValueOf(x).Kind() != reflect.Struct {
		panic(fmt.Errorf("WriteRequest bad param; want struct got %T (%#v)", x, x))
	}
	log.Printf("send client request header: %#v", req)
	if err := c.enc.Encode(req); err != nil {
		return err
	}
	log.Printf("send client request body: %#v", x)
	return c.enc.Encode(x)
}

func (c *generalClientCodec) ReadResponseHeader(resp *rpc.Response) error {
	err := c.dec.Decode(resp)
	log.Printf("got response header %#v", resp)
	return err
}

func (c *generalClientCodec) ReadResponseBody(r interface{}) error {
	if v := reflect.ValueOf(r); v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		panic(fmt.Errorf("ReadResponseBody bad destination; want *struct got %T", r))
	}
	var m json.RawMessage
	err := c.dec.Decode(&m)
	if err != nil {
		return err
	}
	log.Printf("got response body: %q", m)
	err = json.Unmarshal(m, r)
	log.Printf("unmarshalled into %#v", r)
	return err
}

func NewJSONServerCodec(c io.ReadWriteCloser) rpc.ServerCodec {
	return &generalServerCodec{
		Closer: c,
		enc:    json.NewEncoder(c),
		dec:    json.NewDecoder(c),
	}
}

func NewJSONClientCodec(c io.ReadWriteCloser) rpc.ClientCodec {
	return &generalClientCodec{
		Closer: c,
		enc:    json.NewEncoder(c),
		dec:    json.NewDecoder(c),
	}
}
