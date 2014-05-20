// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"reflect"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/rpc/rpcreflect"
	"launchpad.net/juju-core/testing"
)

// We test rpcreflect in this package, so that the
// tests can all share the same testing Root type.

type reflectSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&reflectSuite{})

func (*reflectSuite) TestTypeOf(c *gc.C) {
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

func (*reflectSuite) TestObjTypeOf(c *gc.C) {
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

func (*reflectSuite) TestValueOf(c *gc.C) {
	v := rpcreflect.ValueOf(reflect.ValueOf(nil))
	c.Check(v.IsValid(), jc.IsFalse)
	c.Check(func() { v.MethodCaller("foo", "bar") }, gc.PanicMatches, "MethodCaller called on invalid Value")

	root := &Root{}
	v = rpcreflect.ValueOf(reflect.ValueOf(root))
	c.Check(v.IsValid(), jc.IsTrue)
	c.Check(v.GoValue().Interface(), gc.Equals, root)
}

func (*reflectSuite) TestMethodCaller(c *gc.C) {
	// MethodCaller is actually extensively tested because it's
	// used in the implementation of the rpc server,
	// so just a simple sanity check test here.
	root := &Root{
		simple: make(map[string]*SimpleMethods),
	}
	root.simple["a99"] = &SimpleMethods{root: root, id: "a99"}
	v := rpcreflect.ValueOf(reflect.ValueOf(root))

	m, err := v.MethodCaller("foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unknown object type "foo"`)
	c.Assert(err, gc.FitsTypeOf, (*rpcreflect.CallNotImplementedError)(nil))
	c.Assert(m, gc.DeepEquals, rpcreflect.MethodCaller{})

	m, err = v.MethodCaller("SimpleMethods", "bar")
	c.Assert(err, gc.ErrorMatches, "no such request - method SimpleMethods.bar is not implemented")
	c.Assert(err, gc.FitsTypeOf, (*rpcreflect.CallNotImplementedError)(nil))
	c.Assert(m, gc.DeepEquals, rpcreflect.MethodCaller{})

	m, err = v.MethodCaller("SimpleMethods", "Call1r1e")
	c.Assert(err, gc.IsNil)
	c.Assert(m.ParamsType, gc.Equals, reflect.TypeOf(stringVal{}))
	c.Assert(m.ResultType, gc.Equals, reflect.TypeOf(stringVal{}))

	ret, err := m.Call("a99", reflect.ValueOf(stringVal{"foo"}))
	c.Assert(err, gc.IsNil)
	c.Assert(ret.Interface(), gc.Equals, stringVal{"Call1r1e ret"})
}
