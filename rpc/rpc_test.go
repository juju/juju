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
	stdtesting "testing"
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
	t *testContext
}

func (r *TRoot) A(id string) (*A, error) {
	if id == "" || id[0] != 'a' {
		return nil, fmt.Errorf("unknown a id")
	}
	return r.t.newA(id)
}

type A struct {
	t  *testContext
	id string
}

// Each Call method is named in this standard form:
//
//     Call<narg>r<nret><e>
//
// where narg is the number of arguments, nret is the number of returned
// values (not including the error) and e is the letter 'e' if the
// method returns an error.

func (a *A) Call0r0() {
	a.t.called(a, "Call0r0", nil)
}

func (a *A) Call0r1() stringVal {
	a.t.called(a, "Call0r1", nil)
	return stringVal{"Call0r1 ret"}
}

func (a *A) Call0r1e() (stringVal, error) {
	a.t.called(a, "Call0r1e", nil)
	return stringVal{"Call0r1e ret"}, &callError{a, "Call0r1e", nil}
}

func (a *A) Call0r0e() error {
	a.t.called(a, "Call0r0e", nil)
	return &callError{a, "Call0r0e", nil}
}

func (a *A) Call1r0(s stringVal) {
	a.t.called(a, "Call1r0", s)
}

func (a *A) Call1r1(s stringVal) stringVal {
	a.t.called(a, "Call1r1", s)
	return stringVal{"Call1r1 ret"}
}

func (a *A) Call1r1e(s stringVal) (stringVal, error) {
	a.t.called(a, "Call1r1e", s)
	return stringVal{}, &callError{a, "Call1r1e", s}
}

func (a *A) Call1r0e(s stringVal) error {
	a.t.called(a, "Call1r0e", s)
	return &callError{a, "Call1r0e", s}
}

type testContext struct {
	calls []*callInfo
	as    map[string]*A
}

func (t *testContext) called(rcvr interface{}, method string, arg interface{}) {
	t.calls = append(t.calls, &callInfo{rcvr, method, arg})
}

func (t *testContext) newA(id string) (*A, error) {
	if a := t.as[id]; a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("A(%s) not found", id)
}

func (*suite) TestRPC(c *C) {
	srv, err := rpc.NewServer(&TRoot{})
	c.Assert(err, IsNil)

	l, err := net.Listen("tcp", ":0")
	c.Assert(err, IsNil)
	defer l.Close()

	rootc := make(chan *TRoot)
	srvDone := make(chan error)
	go func() {
		newRoot := func(net.Conn) (interface{}, error) {
			return <-rootc, nil
		}
		err := srv.Accept(l, newRoot, NewJSONServerCodec)
		c.Logf("accept status: %v", err)
		srvDone <- err
	}()

	conn, err := net.Dial("tcp", l.Addr().String())
	c.Assert(err, IsNil)
	defer conn.Close()
	t := &testContext{
		as: map[string]*A{},
	}
	t.as["a99"] = &A{id: "a99", t: t}
	rootc <- &TRoot{t}
	client := rpc.NewClientWithCodec(NewJSONClientCodec(conn))
	for narg := 0; narg < 2; narg++ {
		for nret := 0; nret < 2; nret++ {
			for nerr := 0; nerr < 2; nerr++ {
				t.calls = nil
				t.testCall(c, client, narg, nret, nerr != 0)
			}
		}
	}
	l.Close()
	<-srvDone
}

func (t *testContext) testCall(c *C, client *rpc.Client, narg, nret int, retErr bool) {
	e := ""
	if retErr {
		e = "e"
	}
	method := fmt.Sprintf("Call%dr%d%s", narg, nret, e)
	c.Logf("test call %s", method)
	var r stringVal
	err := client.Call("A", "a99", method, stringVal{"arg"}, &r)
	c.Assert(t.calls, HasLen, 1, Commentf("err %v", err))
	expectCall := callInfo{
		rcvr:   t.as["a99"],
		method: method,
	}
	if narg > 0 {
		expectCall.arg = stringVal{"arg"}
	}
	c.Assert(*t.calls[0], Equals, expectCall)
	switch {
	case retErr:
		c.Assert(err, DeepEquals, &rpc.ServerError{
			fmt.Sprintf("error calling %s", method),
		})
	case nret > 0:
		c.Assert(r, Equals, stringVal{method + " ret"})
	}
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
	if argp == nil {
		argp = &struct{}{}
	}
	return c.dec.Decode(argp)
}

func (c *generalServerCodec) WriteResponse(resp *rpc.Response, v interface{}) error {
	if err := c.enc.Encode(resp); err != nil {
		return err
	}
	return c.enc.Encode(v)
}

type generalClientCodec struct {
	io.Closer
	enc encoder
	dec decoder
}

func (c *generalClientCodec) WriteRequest(req *rpc.Request, x interface{}) error {
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
	var m json.RawMessage
	err := c.dec.Decode(&m)
	if err != nil {
		return err
	}
	log.Printf("got response body: %q", m)
	if r == nil {
		r = &struct{}{}
	}
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
