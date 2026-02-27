// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpcreflect

import (
	"context"
	"reflect"
)

// MethodCaller represents a method that can be called on a facade object.
type MethodCaller interface {
	// ParamsType holds the required type of the parameter to the object method.
	ParamsType() reflect.Type

	// ResultType holds the result type of the result of calling the object method.
	ResultType() reflect.Type

	// Call is actually placing a call to instantiate an given instance and
	// call the method on that instance.
	Call(ctx context.Context, objId string, arg reflect.Value) (reflect.Value, error)
}

// methodCaller knows how to call a particular RPC method.
type methodCaller struct {
	rootValue  reflect.Value
	rootMethod RootMethod
	objMethod  ObjMethod
}

// Value holds the value of the root of an RPC server that
// can call methods directly on a Go value.
type Value struct {
	rootValue reflect.Value
	rootType  *Type
}

// ValueOf returns a value that can be used to call RPC-style
// methods on the given root value. It returns the zero
// Value if rootValue.IsValid is false.
func ValueOf(rootValue reflect.Value) Value {
	if !rootValue.IsValid() {
		return Value{}
	}
	return Value{
		rootValue: rootValue,
		rootType:  TypeOf(rootValue.Type()),
	}
}

// IsValid returns whether the Value has been initialized with ValueOf.
func (v Value) IsValid() bool {
	return v.rootType != nil
}

// GoValue returns the value that was passed to ValueOf to create v.
func (v Value) GoValue() reflect.Value {
	return v.rootValue
}

// FindMethod returns an object that can be used to make calls on
// the given root value to the given root method and object method.
// It returns an error if either the root method or the object
// method were not found.
// It panics if called on the zero Value.
// The version argument is ignored - all versions will find
// the same method.
func (v Value) FindMethod(rootMethodName string, version int, objMethodName string) (MethodCaller, error) {
	if !v.IsValid() {
		panic("FindMethod called on invalid Value")
	}
	caller := methodCaller{
		rootValue: v.rootValue,
	}
	var err error
	caller.rootMethod, err = v.rootType.Method(rootMethodName)
	if err != nil {
		return nil, &CallNotImplementedError{
			RootMethod: rootMethodName,
		}
	}
	caller.objMethod, err = caller.rootMethod.ObjType.Method(objMethodName)
	if err != nil {
		return nil, &CallNotImplementedError{
			RootMethod: rootMethodName,
			Method:     objMethodName,
		}
	}
	return caller, nil
}

// killer is the same interface as rpc.Killer, but redeclared
// here to avoid cyclic dependency.
type killer interface {
	Kill()
}

// Kill implements rpc.Killer.Kill by calling Kill on the root
// value if it implements Killer.
func (v Value) Kill() {
	if killer, ok := v.rootValue.Interface().(killer); ok {
		killer.Kill()
	}
}

// Call implements MethodCaller.Call, which calls the method on the
// root value and then calls the method on the object value.
func (caller methodCaller) Call(ctx context.Context, objId string, arg reflect.Value) (reflect.Value, error) {
	obj, err := caller.rootMethod.Call(caller.rootValue, objId)
	if err != nil {
		return reflect.Value{}, err
	}
	return caller.objMethod.Call(ctx, obj, arg)
}

// ParamsType implements MethodCaller.ParamsType.
func (caller methodCaller) ParamsType() reflect.Type {
	return caller.objMethod.Params
}

// ResultType implements MethodCaller.ResultType.
func (caller methodCaller) ResultType() reflect.Type {
	return caller.objMethod.Result
}
