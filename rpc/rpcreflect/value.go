// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpcreflect

import (
	"fmt"
	"reflect"
)

// CallNotImplementedError is an error, returned an attempt to call to
// an unknown API method is made.
type CallNotImplementedError struct {
	RootMethod string
	Version    int
	Method     string
}

func (e *CallNotImplementedError) Error() string {
	if e.Method == "" {
		if e.Version != 0 {
			return fmt.Sprintf("unknown version (%d) of interface %q", e.Version, e.RootMethod)
		}
		return fmt.Sprintf("unknown object type %q", e.RootMethod)
	}
	methodVersion := e.RootMethod
	if e.Version != 0 {
		methodVersion = fmt.Sprintf("%s(%d)", e.RootMethod, e.Version)
	}
	return fmt.Sprintf("no such request - method %s.%s is not implemented", methodVersion, e.Method)
}

// methodCaller knows how to call a particular RPC method.
type methodCaller struct {
	// paramsType holds the required type of the parameter to the object method.
	paramsType reflect.Type
	// resultType holds the result type of the result of caling the object method.
	resultType reflect.Type

	rootValue  reflect.Value
	rootMethod RootMethod
	objMethod  ObjMethod
}

// methodCaller holds the value of the root of an RPC server that
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
func (v Value) FindMethod(rootMethodName string, version int, objMethodName string) (MethodCaller, error) {
	if !v.IsValid() {
		panic("FindMethod called on invalid Value")
	}
	caller := methodCaller{
		rootValue: v.rootValue,
	}
	if version != 0 {
		return nil, &CallNotImplementedError{
			RootMethod: rootMethodName,
			Version:    version,
		}
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

func (caller methodCaller) Call(objId string, arg reflect.Value) (reflect.Value, error) {
	obj, err := caller.rootMethod.Call(caller.rootValue, objId)
	if err != nil {
		return reflect.Value{}, err
	}
	return caller.objMethod.Call(obj, arg)
}

func (caller methodCaller) ParamsType() reflect.Type {
	return caller.objMethod.Params
}

func (caller methodCaller) ResultType() reflect.Type {
	return caller.objMethod.Result
}

type MethodCaller interface {
	// ParamsType holds the required type of the parameter to the object method.
	ParamsType() reflect.Type

	// ResultType holds the result type of the result of calling the object method.
	ResultType() reflect.Type

	// Call is actually placing a call to instantiate an given instance and
	// call the method on that instance.
	Call(objId string, arg reflect.Value) (reflect.Value, error)
}
