// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"reflect"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/rpc/rpcreflect"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

// We test rpcreflect in this package, so that the
// tests can all share the same testing Root type.

type reflectSuite struct {
	testbase.LoggingSuite
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
		m, ok := rtype.Method(name)
		c.Assert(ok, jc.IsTrue)
		c.Assert(m, gc.NotNil)
		c.Assert(m.Call, gc.NotNil)
		c.Assert(m.ObjType, gc.Equals, rpcreflect.ObjTypeOf(expectGoType))
		c.Assert(m.ObjType.GoType(), gc.Equals, expectGoType)
	}
	m, ok := rtype.Method("not found")
	c.Assert(ok, jc.IsFalse)
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
		m, ok := objType.Method(name)
		c.Check(ok, jc.IsTrue)
		c.Assert(m, gc.NotNil)
		c.Check(m.Call, gc.NotNil)
		c.Check(m.Params, gc.Equals, expectMethod.Params)
		c.Check(m.Result, gc.Equals, expectMethod.Result)
	}
	m, ok := objType.Method("not found")
	c.Check(ok, jc.IsFalse)
	c.Check(m, gc.DeepEquals, rpcreflect.ObjMethod{})
}

// MORE TESTS!
