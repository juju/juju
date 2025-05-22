// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"reflect"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/rpcreflect"
	"github.com/juju/juju/internal/testing"
)

// We test rpcreflect in this package, so that the
// tests can all share the same testing Root type.

type reflectSuite struct {
	testing.BaseSuite
}

func TestReflectSuite(t *stdtesting.T) {
	tc.Run(t, &reflectSuite{})
}

func (*reflectSuite) TestTypeOf(c *tc.C) {
	rtype := rpcreflect.TypeOf(reflect.TypeOf(&Root{}))
	c.Assert(rtype.DiscardedMethods(), tc.DeepEquals, []string{
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
		"ContextMethods":   reflect.TypeOf(&ContextMethods{}),
	}
	c.Assert(rtype.MethodNames(), tc.HasLen, len(expect))
	for name, expectGoType := range expect {
		m, err := rtype.Method(name)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m, tc.NotNil)
		c.Assert(m.Call, tc.NotNil)
		c.Assert(m.ObjType, tc.Equals, rpcreflect.ObjTypeOf(expectGoType))
		c.Assert(m.ObjType.GoType(), tc.Equals, expectGoType)
	}
	m, err := rtype.Method("not found")
	c.Assert(err, tc.Equals, rpcreflect.ErrMethodNotFound)
	c.Assert(m, tc.DeepEquals, rpcreflect.RootMethod{})
}

func (*reflectSuite) TestObjTypeOf(c *tc.C) {
	objType := rpcreflect.ObjTypeOf(reflect.TypeOf(&SimpleMethods{}))
	c.Check(objType.DiscardedMethods(), tc.DeepEquals, []string{
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
	c.Assert(objType.MethodNames(), tc.HasLen, len(expect))
	for name, expectMethod := range expect {
		m, err := objType.Method(name)
		c.Check(err, tc.ErrorIsNil)
		c.Assert(m, tc.NotNil)
		c.Check(m.Call, tc.NotNil)
		c.Check(m.Params, tc.Equals, expectMethod.Params)
		c.Check(m.Result, tc.Equals, expectMethod.Result)
	}
	m, err := objType.Method("not found")
	c.Check(err, tc.Equals, rpcreflect.ErrMethodNotFound)
	c.Check(m, tc.DeepEquals, rpcreflect.ObjMethod{})
}

func (*reflectSuite) TestValueOf(c *tc.C) {
	v := rpcreflect.ValueOf(reflect.ValueOf(nil))
	c.Check(v.IsValid(), tc.IsFalse)
	c.Check(func() { v.FindMethod("foo", 0, "bar") }, tc.PanicMatches, "FindMethod called on invalid Value")

	root := &Root{}
	v = rpcreflect.ValueOf(reflect.ValueOf(root))
	c.Check(v.IsValid(), tc.IsTrue)
	c.Check(v.GoValue().Interface(), tc.Equals, root)
}

func (*reflectSuite) TestFindMethod(c *tc.C) {
	// FindMethod is actually extensively tested because it's
	// used in the implementation of the rpc server,
	// so just a simple sanity check test here.
	root := &Root{
		c:      c,
		simple: make(map[string]*SimpleMethods),
	}
	root.simple["a99"] = &SimpleMethods{root: root, id: "a99"}
	v := rpcreflect.ValueOf(reflect.ValueOf(root))

	m, err := v.FindMethod("foo", 0, "bar")
	c.Assert(err, tc.ErrorMatches, `unknown facade type "foo"`)
	c.Assert(err, tc.FitsTypeOf, (*rpcreflect.CallNotImplementedError)(nil))
	c.Assert(m, tc.IsNil)

	m, err = v.FindMethod("SimpleMethods", 0, "bar")
	c.Assert(err, tc.ErrorMatches, `unknown method "bar" at version 0 for facade type "SimpleMethods"`)
	c.Assert(err, tc.FitsTypeOf, (*rpcreflect.CallNotImplementedError)(nil))
	c.Assert(m, tc.IsNil)

	m, err = v.FindMethod("SimpleMethods", 0, "Call1r1e")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.ParamsType(), tc.Equals, reflect.TypeOf(stringVal{}))
	c.Assert(m.ResultType(), tc.Equals, reflect.TypeOf(stringVal{}))

	ret, err := m.Call(c.Context(), "a99", reflect.ValueOf(stringVal{"foo"}))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ret.Interface(), tc.Equals, stringVal{"Call1r1e ret"})
}

func (*reflectSuite) TestFindMethodAcceptsAnyVersion(c *tc.C) {
	root := &Root{
		c:      c,
		simple: make(map[string]*SimpleMethods),
	}
	root.simple["a99"] = &SimpleMethods{root: root, id: "a99"}
	v := rpcreflect.ValueOf(reflect.ValueOf(root))

	m, err := v.FindMethod("SimpleMethods", 0, "Call1r1e")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.ParamsType(), tc.Equals, reflect.TypeOf(stringVal{}))
	c.Assert(m.ResultType(), tc.Equals, reflect.TypeOf(stringVal{}))

	m, err = v.FindMethod("SimpleMethods", 1, "Call1r1e")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.ParamsType(), tc.Equals, reflect.TypeOf(stringVal{}))
	c.Assert(m.ResultType(), tc.Equals, reflect.TypeOf(stringVal{}))
}
