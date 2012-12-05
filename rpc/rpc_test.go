package rpc_test

import (
	"bytes"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/rpc"
	"net"
	"reflect"
	"testing"
)

type suite struct{}

var _ = Suite(suite{})

func TestAll(t *testing.T) {
	TestingT(t)
}

type TRoot struct {
	A    string
	B    int
	C    *TStruct
	List *TList
	//	D TIntWithMethods TODO
}

type TContext struct {
	S string
}

func (ctxt *TContext) String() string {
	if ctxt == nil {
		return "nil"
	}
	return ctxt.S
}

var contextChecked = make(chan *TContext, 1)

func (TRoot) CheckContext(ctxt *TContext) error {
	contextChecked <- ctxt
	return nil
}

type TList struct {
	Elem string
	Next *TList
}

func (r *TList) MNext() *TList {
	called(nil, "TList.MNext")
	return r.Next
}

type TStruct struct {
	X string
}

func (TStruct) Method_0r0() {
	called(nil, "TStruct.Method_0r0")
}
func (TStruct) Method_c0r0(ctxt *TContext) {
	called(ctxt, "TStruct.Method_c0r0", ctxt)
}
func (TStruct) Method_1r0(x string) {
	called(nil, "TStruct.Method_1r0", x)
}
func (TStruct) Method_1r1(x string) string {
	called(nil, "TStruct.Method_1r1", x)
	return "TStruct.Method_1r1"
}
func (TStruct) Method_c1r0(ctxt *TContext, x string) {
	called(ctxt, "TStruct.Method_c1r0", ctxt, x)
}
func (TStruct) Method_c2r0(ctxt *TContext, x, bogus string) {
	called(ctxt, "TStruct.Method_c2r0", ctxt, x, bogus)
}
func (TStruct) Method_0r0e() error {
	called(nil, "TStruct.Method_0r0e")
	return methodError("TStruct.Method_0r0e")
}
func (TStruct) Method_0r1() string {
	called(nil, "TStruct.Method_0r1")
	return "TStruct.Method_0r0e"
}
func (TStruct) Method_0r1e() (string, error) {
	called(nil, "TStruct.Method_0r1e")
	return "TStruct.Method_0r1e", nil
}

// TODO fill in other permutations

func (TStruct) Method_c1r1e(ctxt *TContext, x string) (int, error) {
	called(ctxt, "TStruct.Method_c1r1e", x)
	return 0, methodError("TStruct.Method_c1r1e")
}

func methodError(name string) error {
	return fmt.Errorf("method %s", name)
}

var root = &TRoot{
	A: "A",
	B: 99,
	C: &TStruct{
		X: "X",
	},
	List: newList("zero", "one", "two"),
}

func newList(elems ...string) *TList {
	var l *TList
	for i := len(elems) - 1; i >= 0; i-- {
		l = &TList{
			Elem: elems[i],
			Next: l,
		}
	}
	return l
}

var calls []string

func called(ctxt *TContext, args ...interface{}) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%v:", ctxt)
	for _, a := range args {
		fmt.Fprintf(&b, " %v", a)
	}
	calls = append(calls, b.String())
}

var tests = []struct {
	path    string
	calls   []string
	arg     interface{}
	ret     interface{}
	err     string
	errPath string
}{{
	path: "/A",
	ret:  "A",
}, {
	path:    "/A/B",
	err:     "not found",
	errPath: "A/B",
}, {
	path: "/B",
	ret:  99,
}, {
	path: "/C",
	ret:  &TStruct{X: "X"},
}, {
	path: "/C/X",
	ret:  "X",
}, {
	path:  "/C/Method_0r0",
	calls: []string{"nil: TStruct.Method_0r0"},
}, {
	path:  "/C/Method_c1r1e",
	arg:   "hello",
	calls: []string{"ctxt: TStruct.Method_c1r1e hello"},
	err:   "method TStruct.Method_c1r1e",
}, {
	path:  "/C/Method_1r1",
	arg:   "hello",
	calls: []string{"nil: TStruct.Method_1r1 hello"},
	ret:   "TStruct.Method_1r1",
}, {
	path:  "/C/Method_c1r1e-hello",
	calls: []string{"ctxt: TStruct.Method_c1r1e hello"},
	err:   "method TStruct.Method_c1r1e",
}, {
	path: "/List",
	ret:  newList("zero", "one", "two"),
}, {
	path: "/List/Next",
	ret:  newList("one", "two"),
}, {
	path: "/List/Next/Next",
	ret:  newList("two"),
}, {
	path:  "/List/MNext",
	ret:   newList("one", "two"),
	calls: []string{"nil: TList.MNext"},
}, {
	path:  "/List/MNext/MNext",
	ret:   newList("two"),
	calls: []string{"nil: TList.MNext", "nil: TList.MNext"},
},

//{
//	path: "/List/Next/Next/Next",
//}, 
}

func (suite) TestServeCodec(c *C) {
	ctxt := &TContext{"ctxt"}
	srv, err := rpc.NewServer(root)
	c.Assert(err, IsNil)
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.path)
		calls = nil

		codec := &singleShotCodec{
			req: rpc.Request{
				Path: test.path,
				Seq:  99,
			},
			reqBody: test.arg,
		}
		done := make(chan error)
		go func() {
			done <- srv.ServeCodec(codec, ctxt)
		}()
		c.Assert(<-contextChecked, Equals, ctxt)
		err := <-done
		c.Assert(err, IsNil)
		c.Assert(codec.doneHeader, Equals, true)
		c.Assert(codec.doneBody, Equals, true)
		c.Assert(codec.resp.Seq, Equals, uint64(99))
		c.Assert(calls, DeepEquals, test.calls)
		if test.err != "" {
			c.Assert(codec.resp.Error, Matches, test.err)
			c.Assert(codec.resp.ErrorPath, Matches, test.errPath)
			continue
		}
		c.Assert(codec.resp.Error, Equals, "")
		c.Assert(codec.resp.ErrorPath, Equals, "")
		c.Assert(codec.respValue, DeepEquals, test.ret)
	}
}

type codecInfo struct {
	name      string
	newClient func(io.ReadWriter) rpc.ClientCodec
	newServer func(io.ReadWriter) rpc.ServerCodec
}

var codecs = []codecInfo{{
	"json",
	rpc.NewJSONClientCodec,
	rpc.NewJSONServerCodec,
},

// XML doesn't currently work because it doesn't delimit messages.
//{
//	"xml",
//	rpc.NewXMLClientCodec,
//	rpc.NewXMLServerCodec,
//},
}

func (suite) TestClientCodecs(c *C) {

	for i, info := range codecs {
		c.Logf("test %d. %s", i, info.name)
		testCodec(c, info)
	}
}

func testCodec(c *C, info codecInfo) {
	ctxt := &TContext{"ctxt"}
	srv, err := rpc.NewServer(root)
	c.Assert(err, IsNil)

	l, err := net.Listen("tcp", ":0")
	c.Assert(err, IsNil)
	defer l.Close()

	srvDone := make(chan error)
	go func() {
		// TODO better checking of context.
		err := srv.Accept(l, info.newServer, func(net.Conn) interface{} {
			return ctxt
		})
		c.Logf("Accept returned %v", err)
		srvDone <- err
	}()

	conn, err := net.Dial("tcp", l.Addr().String())
	c.Assert(err, IsNil)
	defer conn.Close()
	c.Assert(<-contextChecked, Equals, ctxt)
	client := rpc.NewClientWithCodec(info.newClient(conn))
	for i, test := range tests {
		c.Logf("test %s.%d: %s", info.name, i, test.path)
		calls = nil
		var ret interface{}
		if test.ret != nil {
			ret = reflect.New(reflect.TypeOf(test.ret)).Interface()
		}
		err := client.Call(test.path, test.arg, ret)
		c.Assert(calls, DeepEquals, test.calls)
		if test.err != "" {
			if test.errPath != "" {
				c.Assert(err, ErrorMatches, fmt.Sprintf("error at %q: %s", test.errPath, test.err))
			} else {
				c.Assert(err, ErrorMatches, test.err)
			}
			continue
		}
		c.Assert(err, IsNil)
		if test.ret != nil {
			c.Assert(reflect.ValueOf(ret).Elem().Interface(), DeepEquals, test.ret)
		}
	}
	l.Close()
	c.Logf("Accept status: %v", <-srvDone)
}

//func (suite) TestHTTPCodec(c *C) {
//	ctxt := &TContext{"ctxt"}
//	srv, err := rpc.NewServer(root)
//	c.Assert(err, IsNil)
//}

type singleShotCodec struct {
	req     rpc.Request
	reqBody interface{}

	resp      rpc.Response
	respValue interface{}

	doneHeader bool
	doneBody   bool
}

func (c *singleShotCodec) ReadRequestHeader(req *rpc.Request) error {
	if c.doneHeader {
		return io.EOF
	}
	*req = c.req
	c.doneHeader = true
	return nil
}

func (c *singleShotCodec) ReadRequestBody(argp interface{}) error {
	if c.doneBody {
		panic("readBody called twice")
	}
	c.doneBody = true
	if argp == nil {
		return nil
	}
	v := reflect.ValueOf(argp)
	t := v.Type()
	if t.Kind() != reflect.Ptr {
		return fmt.Errorf("want pointer, got %s", t)
	}
	bodyv := reflect.ValueOf(c.reqBody)
	if t.Elem() != bodyv.Type() {
		return fmt.Errorf("expected type %s, got %s", bodyv.Type(), t.Elem())
	}
	v.Elem().Set(bodyv)
	return nil
}

func (c *singleShotCodec) WriteResponse(resp *rpc.Response, v interface{}) error {
	if !c.doneHeader {
		panic("header not read")
	}
	if !c.doneBody {
		panic("body not read")
	}
	c.resp = *resp
	c.respValue = v
	return nil
}
