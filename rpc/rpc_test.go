// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"sync"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/rpc/jsoncodec"
	"launchpad.net/juju-core/rpc/rpcreflect"
	"launchpad.net/juju-core/testing/testbase"
)

// We also test rpc/rpcreflect in this package, so that the
// tests can all share the same testing Root type.

type suite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&suite{})

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
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

type Root struct {
	mu        sync.Mutex
	conn      *rpc.Conn
	calls     []*callInfo
	returnErr bool
	simple    map[string]*SimpleMethods
	delayed   map[string]*DelayedMethods
	errorInst *ErrorMethods
}

func (r *Root) callError(rcvr interface{}, name string, arg interface{}) error {
	if r.returnErr {
		return &callError{rcvr, name, arg}
	}
	return nil
}

func (r *Root) SimpleMethods(id string) (*SimpleMethods, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.simple[id]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("unknown SimpleMethods id")
}

func (r *Root) DelayedMethods(id string) (*DelayedMethods, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.delayed[id]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("unknown DelayedMethods id")
}

func (r *Root) ErrorMethods(id string) (*ErrorMethods, error) {
	if r.errorInst == nil {
		return nil, fmt.Errorf("no error methods")
	}
	return r.errorInst, nil
}

func (r *Root) Discard1() {}

func (r *Root) Discard2(id string) error { return nil }

func (r *Root) Discard3(id string) int { return 0 }

func (r *Root) CallbackMethods(string) (*CallbackMethods, error) {
	return &CallbackMethods{r}, nil
}

func (r *Root) InterfaceMethods(id string) (InterfaceMethods, error) {
	log.Infof("interface methods called")
	m, err := r.SimpleMethods(id)
	if err != nil {
		return nil, err
	}
	return m, nil
}

type InterfaceMethods interface {
	Call1r1e(s stringVal) (stringVal, error)
}

type ChangeAPIMethods struct {
	r *Root
}

func (r *Root) ChangeAPIMethods(string) (*ChangeAPIMethods, error) {
	return &ChangeAPIMethods{r}, nil
}

func (t *Root) called(rcvr interface{}, method string, arg interface{}) {
	t.mu.Lock()
	t.calls = append(t.calls, &callInfo{rcvr, method, arg})
	t.mu.Unlock()
}

type SimpleMethods struct {
	root *Root
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

func (a *SimpleMethods) SliceArg(struct{ X []string }) stringVal {
	return stringVal{"SliceArg ret"}
}

func (a *SimpleMethods) Discard1(int) {}

func (a *SimpleMethods) Discard2(struct{}, struct{}) {}

func (a *SimpleMethods) Discard3() int { return 0 }

func (a *SimpleMethods) Discard4() (_, _ struct{}) { return }

type DelayedMethods struct {
	ready     chan struct{}
	done      chan string
	doneError chan error
}

func (a *DelayedMethods) Delay() (stringVal, error) {
	if a.ready != nil {
		a.ready <- struct{}{}
	}
	select {
	case s := <-a.done:
		return stringVal{s}, nil
	case err := <-a.doneError:
		return stringVal{}, err
	}
}

type ErrorMethods struct {
	err error
}

func (e *ErrorMethods) Call() error {
	return e.err
}

type CallbackMethods struct {
	root *Root
}

type int64val struct {
	I int64
}

func (a *CallbackMethods) Factorial(x int64val) (int64val, error) {
	if x.I <= 1 {
		return int64val{1}, nil
	}
	var r int64val
	err := a.root.conn.Call("CallbackMethods", "", "Factorial", int64val{x.I - 1}, &r)
	if err != nil {
		return int64val{}, err
	}
	return int64val{x.I * r.I}, nil
}

func (a *ChangeAPIMethods) ChangeAPI() {
	a.r.conn.Serve(&changedAPIRoot{}, nil)
}

func (a *ChangeAPIMethods) RemoveAPI() {
	a.r.conn.Serve(nil, nil)
}

type changedAPIRoot struct{}

func (r *changedAPIRoot) NewlyAvailable(string) (newlyAvailableMethods, error) {
	return newlyAvailableMethods{}, nil
}

type newlyAvailableMethods struct{}

func (newlyAvailableMethods) NewMethod() stringVal {
	return stringVal{"new method result"}
}

func (*suite) TestRPC(c *gc.C) {
	root := &Root{
		simple: make(map[string]*SimpleMethods),
	}
	root.simple["a99"] = &SimpleMethods{root: root, id: "a99"}
	client, srvDone := newRPCClientServer(c, root, nil, false)
	defer closeClient(c, client, srvDone)
	for narg := 0; narg < 2; narg++ {
		for nret := 0; nret < 2; nret++ {
			for nerr := 0; nerr < 2; nerr++ {
				retErr := nerr != 0
				root.testCall(c, client, "SimpleMethods", narg, nret, retErr, false)
				if retErr {
					root.testCall(c, client, "SimpleMethods", narg, nret, retErr, true)
				}
			}
		}
	}
}

func callName(narg, nret int, retErr bool) string {
	e := ""
	if retErr {
		e = "e"
	}
	return fmt.Sprintf("Call%dr%d%s", narg, nret, e)
}

func (root *Root) testCall(c *gc.C, conn *rpc.Conn, entry string, narg, nret int, retErr, testErr bool) {
	root.calls = nil
	root.returnErr = testErr
	method := callName(narg, nret, retErr)
	c.Logf("test call %s", method)
	var r stringVal
	err := conn.Call(entry, "a99", method, stringVal{"arg"}, &r)
	root.mu.Lock()
	defer root.mu.Unlock()
	expectCall := callInfo{
		rcvr:   root.simple["a99"],
		method: method,
	}
	if narg > 0 {
		expectCall.arg = stringVal{"arg"}
	}
	c.Assert(root.calls, gc.HasLen, 1)
	c.Assert(*root.calls[0], gc.Equals, expectCall)
	switch {
	case retErr && testErr:
		c.Assert(err, gc.DeepEquals, &rpc.RequestError{
			Message: fmt.Sprintf("error calling %s", method),
		})
		c.Assert(r, gc.Equals, stringVal{})
	case nret > 0:
		c.Assert(r, gc.Equals, stringVal{method + " ret"})
	}
}

func (*suite) TestInterfaceMethods(c *gc.C) {
	root := &Root{
		simple: make(map[string]*SimpleMethods),
	}
	root.simple["a99"] = &SimpleMethods{root: root, id: "a99"}
	client, srvDone := newRPCClientServer(c, root, nil, false)
	defer closeClient(c, client, srvDone)
	root.testCall(c, client, "InterfaceMethods", 1, 1, true, false)
	root.testCall(c, client, "InterfaceMethods", 1, 1, true, true)
}

func (*suite) TestRootInfo(c *gc.C) {
	rtype := rpcreflect.TypeOf(reflect.TypeOf(&Root{}))
	c.Assert(rtype.DiscardedMethods(), gc.DeepEquals, []string{
		"Discard1",
		"Discard2",
		"Discard3",
	})
	expect := map[string]reflect.Type{
		"CallbackMethods":  reflect.TypeOf(&CallbackMethods{}),
		"ChangeAPIMethods": reflect.TypeOf(&ChangeAPIMethods{}),
		"DelayedMethods":   reflect.TypeOf(&DelayedMethods{}),
		"ErrorMethods":     reflect.TypeOf(&ErrorMethods{}),
		"InterfaceMethods": reflect.TypeOf((*InterfaceMethods)(nil)).Elem(),
		"SimpleMethods":    reflect.TypeOf(&SimpleMethods{}),
	}
	c.Assert(rtype.MethodNames(), gc.HasLen, len(expect))
	for name, expectGoType := range expect {
		m, err := rtype.Method(name)
		c.Assert(err, gc.IsNil)
		c.Assert(m, gc.NotNil)
		c.Assert(m.Call, gc.NotNil)
		c.Assert(m.ObjType, gc.Equals, rpcreflect.ObjTypeOf(expectGoType))
		c.Assert(m.ObjType.GoType(), gc.Equals, expectGoType)
	}
	m, err := rtype.Method("not found")
	c.Assert(err, gc.Equals, rpcreflect.ErrMethodNotFound)
	c.Assert(m, gc.DeepEquals, rpcreflect.RootMethod{})
}

func (*suite) TestObjectInfo(c *gc.C) {
	objType := rpcreflect.ObjTypeOf(reflect.TypeOf(&SimpleMethods{}))
	c.Check(objType.DiscardedMethods(), gc.DeepEquals, []string{
		"Discard1",
		"Discard2",
		"Discard3",
		"Discard4",
	})
	expect := map[string]*rpcreflect.ObjMethod{
		"SliceArg": {
			Params: reflect.TypeOf(struct{ X []string }{}),
			Result: reflect.TypeOf(stringVal{}),
		},
	}
	for narg := 0; narg < 2; narg++ {
		for nret := 0; nret < 2; nret++ {
			for nerr := 0; nerr < 2; nerr++ {
				retErr := nerr != 0
				var m rpcreflect.ObjMethod
				if narg > 0 {
					m.Params = reflect.TypeOf(stringVal{})
				}
				if nret > 0 {
					m.Result = reflect.TypeOf(stringVal{})
				}
				expect[callName(narg, nret, retErr)] = &m
			}
		}
	}
	c.Assert(objType.MethodNames(), gc.HasLen, len(expect))
	for name, expectMethod := range expect {
		m, err := objType.Method(name)
		c.Check(err, gc.IsNil)
		c.Assert(m, gc.NotNil)
		c.Check(m.Call, gc.NotNil)
		c.Check(m.Params, gc.Equals, expectMethod.Params)
		c.Check(m.Result, gc.Equals, expectMethod.Result)
	}
	m, err := objType.Method("not found")
	c.Check(err, gc.Equals, rpcreflect.ErrMethodNotFound)
	c.Check(m, gc.DeepEquals, rpcreflect.ObjMethod{})
}

func (*suite) TestConcurrentCalls(c *gc.C) {
	start1 := make(chan string)
	start2 := make(chan string)
	ready1 := make(chan struct{})
	ready2 := make(chan struct{})

	root := &Root{
		delayed: map[string]*DelayedMethods{
			"1": {ready: ready1, done: start1},
			"2": {ready: ready2, done: start2},
		},
	}

	client, srvDone := newRPCClientServer(c, root, nil, false)
	defer closeClient(c, client, srvDone)
	call := func(id string, done chan<- struct{}) {
		var r stringVal
		err := client.Call("DelayedMethods", id, "Delay", nil, &r)
		c.Check(err, gc.IsNil)
		c.Check(r.Val, gc.Equals, "return "+id)
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

func (*suite) TestErrorCode(c *gc.C) {
	root := &Root{
		errorInst: &ErrorMethods{&codedError{"message", "code"}},
	}
	client, srvDone := newRPCClientServer(c, root, nil, false)
	defer closeClient(c, client, srvDone)
	err := client.Call("ErrorMethods", "", "Call", nil, nil)
	c.Assert(err, gc.ErrorMatches, `request error: message \(code\)`)
	c.Assert(err.(rpc.ErrorCoder).ErrorCode(), gc.Equals, "code")
}

func (*suite) TestTransformErrors(c *gc.C) {
	root := &Root{
		errorInst: &ErrorMethods{&codedError{"message", "code"}},
	}
	tfErr := func(err error) error {
		c.Check(err, gc.NotNil)
		if e, ok := err.(*codedError); ok {
			return &codedError{
				m:    "transformed: " + e.m,
				code: "transformed: " + e.code,
			}
		}
		return fmt.Errorf("transformed: %v", err)
	}
	client, srvDone := newRPCClientServer(c, root, tfErr, false)
	defer closeClient(c, client, srvDone)
	err := client.Call("ErrorMethods", "", "Call", nil, nil)
	c.Assert(err, gc.DeepEquals, &rpc.RequestError{
		Message: "transformed: message",
		Code:    "transformed: code",
	})

	root.errorInst.err = nil
	err = client.Call("ErrorMethods", "", "Call", nil, nil)
	c.Assert(err, gc.IsNil)

	root.errorInst = nil
	err = client.Call("ErrorMethods", "", "Call", nil, nil)
	c.Assert(err, gc.DeepEquals, &rpc.RequestError{
		Message: "transformed: no error methods",
	})

}

func (*suite) TestServerWaitsForOutstandingCalls(c *gc.C) {
	ready := make(chan struct{})
	start := make(chan string)
	root := &Root{
		delayed: map[string]*DelayedMethods{
			"1": {
				ready: ready,
				done:  start,
			},
		},
	}
	client, srvDone := newRPCClientServer(c, root, nil, false)
	defer closeClient(c, client, srvDone)
	done := make(chan struct{})
	go func() {
		var r stringVal
		err := client.Call("DelayedMethods", "1", "Delay", nil, &r)
		c.Check(err, gc.Equals, rpc.ErrShutdown)
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
}

func chanRead(c *gc.C, ch <-chan struct{}, what string) {
	select {
	case <-ch:
		return
	case <-time.After(3 * time.Second):
		c.Fatalf("timeout on channel read %s", what)
	}
}

func (*suite) TestCompatibility(c *gc.C) {
	root := &Root{
		simple: make(map[string]*SimpleMethods),
	}
	a0 := &SimpleMethods{root: root, id: "a0"}
	root.simple["a0"] = a0

	client, srvDone := newRPCClientServer(c, root, nil, false)
	defer closeClient(c, client, srvDone)
	call := func(method string, arg, ret interface{}) (passedArg interface{}) {
		root.calls = nil
		err := client.Call("SimpleMethods", "a0", method, arg, ret)
		c.Assert(err, gc.IsNil)
		c.Assert(root.calls, gc.HasLen, 1)
		info := root.calls[0]
		c.Assert(info.rcvr, gc.Equals, a0)
		c.Assert(info.method, gc.Equals, method)
		return info.arg
	}
	type extra struct {
		Val   string
		Extra string
	}
	// Extra fields in request and response.
	var r extra
	arg := call("Call1r1", extra{"x", "y"}, &r)
	c.Assert(arg, gc.Equals, stringVal{"x"})

	// Nil argument as request.
	r = extra{}
	arg = call("Call1r1", nil, &r)
	c.Assert(arg, gc.Equals, stringVal{})

	// Nil argument as response.
	arg = call("Call1r1", stringVal{"x"}, nil)
	c.Assert(arg, gc.Equals, stringVal{"x"})

	// Non-nil argument for no response.
	r = extra{}
	arg = call("Call1r0", stringVal{"x"}, &r)
	c.Assert(arg, gc.Equals, stringVal{"x"})
	c.Assert(r, gc.Equals, extra{})
}

func (*suite) TestBadCall(c *gc.C) {
	root := &Root{
		simple: make(map[string]*SimpleMethods),
	}
	a0 := &SimpleMethods{root: root, id: "a0"}
	root.simple["a0"] = a0
	client, srvDone := newRPCClientServer(c, root, nil, false)
	defer closeClient(c, client, srvDone)

	err := client.Call("BadSomething", "a0", "No", nil, nil)
	c.Assert(err, gc.ErrorMatches, `request error: unknown object type "BadSomething"`)

	err = client.Call("SimpleMethods", "xx", "No", nil, nil)
	c.Assert(err, gc.ErrorMatches, `request error: no such request "No" on SimpleMethods`)

	err = client.Call("SimpleMethods", "xx", "Call0r0", nil, nil)
	c.Assert(err, gc.ErrorMatches, "request error: unknown SimpleMethods id")
}

func (*suite) TestContinueAfterReadBodyError(c *gc.C) {
	root := &Root{
		simple: make(map[string]*SimpleMethods),
	}
	a0 := &SimpleMethods{root: root, id: "a0"}
	root.simple["a0"] = a0
	client, srvDone := newRPCClientServer(c, root, nil, false)
	defer closeClient(c, client, srvDone)

	var ret stringVal
	arg0 := struct {
		X map[string]int
	}{
		X: map[string]int{"hello": 65},
	}
	err := client.Call("SimpleMethods", "a0", "SliceArg", arg0, &ret)
	c.Assert(err, gc.ErrorMatches, `request error: json: cannot unmarshal object into Go value of type \[\]string`)

	err = client.Call("SimpleMethods", "a0", "SliceArg", arg0, &ret)
	c.Assert(err, gc.ErrorMatches, `request error: json: cannot unmarshal object into Go value of type \[\]string`)

	arg1 := struct {
		X []string
	}{
		X: []string{"one"},
	}
	err = client.Call("SimpleMethods", "a0", "SliceArg", arg1, &ret)
	c.Assert(err, gc.IsNil)
	c.Assert(ret.Val, gc.Equals, "SliceArg ret")
}

func (*suite) TestErrorAfterClientClose(c *gc.C) {
	client, srvDone := newRPCClientServer(c, &Root{}, nil, false)
	err := client.Close()
	c.Assert(err, gc.IsNil)
	err = client.Call("Foo", "", "Bar", nil, nil)
	c.Assert(err, gc.Equals, rpc.ErrShutdown)
	err = chanReadError(c, srvDone, "server done")
	c.Assert(err, gc.IsNil)
}

func (*suite) TestClientCloseIdempotent(c *gc.C) {
	client, _ := newRPCClientServer(c, &Root{}, nil, false)
	err := client.Close()
	c.Assert(err, gc.IsNil)
	err = client.Close()
	c.Assert(err, gc.IsNil)
	err = client.Close()
	c.Assert(err, gc.IsNil)
}

type KillerRoot struct {
	killed bool
	Root
}

func (r *KillerRoot) Kill() {
	r.killed = true
}

func (*suite) TestRootIsKilled(c *gc.C) {
	root := &KillerRoot{}
	client, srvDone := newRPCClientServer(c, root, nil, false)
	err := client.Close()
	c.Assert(err, gc.IsNil)
	err = chanReadError(c, srvDone, "server done")
	c.Assert(err, gc.IsNil)
	c.Assert(root.killed, gc.Equals, true)
}

func (*suite) TestBidirectional(c *gc.C) {
	srvRoot := &Root{}
	client, srvDone := newRPCClientServer(c, srvRoot, nil, true)
	defer closeClient(c, client, srvDone)
	clientRoot := &Root{conn: client}
	client.Serve(clientRoot, nil)
	var r int64val
	err := client.Call("CallbackMethods", "", "Factorial", int64val{12}, &r)
	c.Assert(err, gc.IsNil)
	c.Assert(r.I, gc.Equals, int64(479001600))
}

func (*suite) TestServerRequestWhenNotServing(c *gc.C) {
	srvRoot := &Root{}
	client, srvDone := newRPCClientServer(c, srvRoot, nil, true)
	defer closeClient(c, client, srvDone)
	var r int64val
	err := client.Call("CallbackMethods", "", "Factorial", int64val{12}, &r)
	c.Assert(err, gc.ErrorMatches, "request error: request error: no service")
}

func (*suite) TestChangeAPI(c *gc.C) {
	srvRoot := &Root{}
	client, srvDone := newRPCClientServer(c, srvRoot, nil, true)
	defer closeClient(c, client, srvDone)
	var s stringVal
	err := client.Call("NewlyAvailable", "", "NewMethod", nil, &s)
	c.Assert(err, gc.ErrorMatches, `request error: unknown object type "NewlyAvailable"`)
	err = client.Call("ChangeAPIMethods", "", "ChangeAPI", nil, nil)
	c.Assert(err, gc.IsNil)
	err = client.Call("ChangeAPIMethods", "", "ChangeAPI", nil, nil)
	c.Assert(err, gc.ErrorMatches, `request error: unknown object type "ChangeAPIMethods"`)
	err = client.Call("NewlyAvailable", "", "NewMethod", nil, &s)
	c.Assert(err, gc.IsNil)
	c.Assert(s, gc.Equals, stringVal{"new method result"})
}

func (*suite) TestChangeAPIToNil(c *gc.C) {
	srvRoot := &Root{}
	client, srvDone := newRPCClientServer(c, srvRoot, nil, true)
	defer closeClient(c, client, srvDone)

	err := client.Call("ChangeAPIMethods", "", "RemoveAPI", nil, nil)
	c.Assert(err, gc.IsNil)

	err = client.Call("ChangeAPIMethods", "", "RemoveAPI", nil, nil)
	c.Assert(err, gc.ErrorMatches, "request error: no service")
}

func (*suite) TestChangeAPIWhileServingRequest(c *gc.C) {
	ready := make(chan struct{})
	done := make(chan error)
	srvRoot := &Root{
		delayed: map[string]*DelayedMethods{
			"1": {ready: ready, doneError: done},
		},
	}
	transform := func(err error) error {
		return fmt.Errorf("transformed: %v", err)
	}
	client, srvDone := newRPCClientServer(c, srvRoot, transform, true)
	defer closeClient(c, client, srvDone)

	result := make(chan error)
	go func() {
		result <- client.Call("DelayedMethods", "1", "Delay", nil, nil)
	}()
	chanRead(c, ready, "method ready")

	err := client.Call("ChangeAPIMethods", "", "ChangeAPI", nil, nil)
	c.Assert(err, gc.IsNil)

	// Ensure that not only does the request in progress complete,
	// but that the original transformErrors function is called.
	done <- fmt.Errorf("an error")
	select {
	case r := <-result:
		c.Assert(r, gc.ErrorMatches, "request error: transformed: an error")
	case <-time.After(3 * time.Second):
		c.Fatalf("timeout on channel read")
	}
}

func chanReadError(c *gc.C, ch <-chan error, what string) error {
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
// it sends a value on the returned channel.
// If bidir is true, requests can flow in both directions.
func newRPCClientServer(c *gc.C, root interface{}, tfErr func(error) error, bidir bool) (*rpc.Conn, <-chan error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, gc.IsNil)

	srvDone := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			srvDone <- nil
			return
		}
		defer l.Close()
		role := roleServer
		if bidir {
			role = roleBoth
		}
		rpcConn := rpc.NewConn(NewJSONCodec(conn, role))
		rpcConn.Serve(root, tfErr)
		if root, ok := root.(*Root); ok {
			root.conn = rpcConn
		}
		rpcConn.Start()
		<-rpcConn.Dead()
		srvDone <- rpcConn.Close()
	}()
	conn, err := net.Dial("tcp", l.Addr().String())
	c.Assert(err, gc.IsNil)
	role := roleClient
	if bidir {
		role = roleBoth
	}
	client := rpc.NewConn(NewJSONCodec(conn, role))
	client.Start()
	return client, srvDone
}

func closeClient(c *gc.C, client *rpc.Conn, srvDone <-chan error) {
	err := client.Close()
	c.Assert(err, gc.IsNil)
	err = chanReadError(c, srvDone, "server done")
	c.Assert(err, gc.IsNil)
}

type encoder interface {
	Encode(e interface{}) error
}

type decoder interface {
	Decode(e interface{}) error
}

// testCodec wraps an rpc.Codec with extra error checking code.
type testCodec struct {
	role connRole
	rpc.Codec
}

func (c *testCodec) WriteMessage(hdr *rpc.Header, x interface{}) error {
	if reflect.ValueOf(x).Kind() != reflect.Struct {
		panic(fmt.Errorf("WriteRequest bad param; want struct got %T (%#v)", x, x))
	}
	if c.role != roleBoth && hdr.IsRequest() != (c.role == roleClient) {
		panic(fmt.Errorf("codec role %v; header wrong type %#v", c.role, hdr))
	}
	log.Infof("send header: %#v; body: %#v", hdr, x)
	return c.Codec.WriteMessage(hdr, x)
}

func (c *testCodec) ReadHeader(hdr *rpc.Header) error {
	err := c.Codec.ReadHeader(hdr)
	if err != nil {
		return err
	}
	log.Infof("got header %#v", hdr)
	if c.role != roleBoth && hdr.IsRequest() == (c.role == roleClient) {
		panic(fmt.Errorf("codec role %v; read wrong type %#v", c.role, hdr))
	}
	return nil
}

func (c *testCodec) ReadBody(r interface{}, isRequest bool) error {
	if v := reflect.ValueOf(r); v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		panic(fmt.Errorf("ReadResponseBody bad destination; want *struct got %T", r))
	}
	if c.role != roleBoth && isRequest == (c.role == roleClient) {
		panic(fmt.Errorf("codec role %v; read wrong body type %#v", c.role, r))
	}
	// Note: this will need to change if we want to test a non-JSON codec.
	var m json.RawMessage
	err := c.Codec.ReadBody(&m, isRequest)
	if err != nil {
		return err
	}
	log.Infof("got response body: %q", m)
	err = json.Unmarshal(m, r)
	log.Infof("unmarshalled into %#v", r)
	return err
}

type connRole string

const (
	roleBoth   connRole = "both"
	roleClient connRole = "client"
	roleServer connRole = "server"
)

func NewJSONCodec(c net.Conn, role connRole) rpc.Codec {
	return &testCodec{
		role:  role,
		Codec: jsoncodec.NewNet(c),
	}
}
