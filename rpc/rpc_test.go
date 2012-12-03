package rpc_test
import (
	. "launchpad.net/gocheck"
	"testing"
	"fmt"
	"launchpad.net/juju-core/rpc"
	"reflect"
	"strings"
)

type suite struct{}
var _ = Suite(suite{})

func TestAll(t *testing.T) {
	TestingT(t)
}

type TRoot struct {
	A string
	B int
	C TStruct
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

func (TRoot) CheckContext(ctxt *TContext) error {
	called(ctxt, "TRoot.CheckContext ")
	return nil
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
	called(nil, "TStruct.Method1r0", x)
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
	C: TStruct{
		X: "X",
	},
}

var calls []string
func called(ctxt *TContext, args ...interface{}) {
	calls = append(calls, fmt.Sprintf("%v: %s", ctxt, fmt.Sprint(args)))
}

var tests = []struct{
	path string
	calls string
	arg interface{}
	ret interface{}
	err string
}{{
	path: "/A",
	ret: "A",
}, {
	path: "/B",
	ret: 99,
}, {
	path: "/C",
	ret: TStruct{X: "X"},
},  {
	path: "/C/X",
	ret: "X",
}, {
	path: "/C/Method_0r0",
	calls: "nil: TStruct.Method_0r0",
}, {
	path: "/C/Method_c1r1e",
	arg: "hello",
	calls: "ctxt: TStruct.Method_c1r1e hello",
	err: "method TStruct.Method_c1r1e",
}, {
	path: "/C/Method_c1r1e-hello",
	calls: "ctxt: TStruct.Method_c1r1e hello",
	err: "method TStruct.Method_c1r1e",
},
}

func (suite) TestCall(c *C) {
	ctxt := &TContext{"ctxt"}
	srv, err := rpc.NewServer(root)
	c.Assert(err, IsNil)
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.path)
		calls = nil
		v, err := srv.Call(test.path, ctxt, reflect.ValueOf(test.arg))
		if test.err != "" {
			c.Assert(err, ErrorMatches, test.err)
		} else {
			c.Assert(err, IsNil)
			c.Assert(strings.Join(calls, "; "), Equals, "TRoot.CheckContext; " + test.calls)
			if test.ret == nil {
				c.Assert(v.IsValid(), Equals, false)
			} else {
				c.Assert(v.Interface(), DeepEquals, test.ret)
			}
		}
	}
}