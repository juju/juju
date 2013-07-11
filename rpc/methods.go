// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"fmt"
	"reflect"
	"sync"

	"launchpad.net/juju-core/log"
)

var (
	errorType  = reflect.TypeOf((*error)(nil)).Elem()
	stringType = reflect.TypeOf("")
)

var (
	typeMutex     sync.RWMutex
	methodsByType = make(map[reflect.Type]*serverMethods)
)

// serverMethods holds information about a type that
// implements RPC server methods.
type serverMethods struct {
	// root holds the root type.
	root reflect.Type

	// obtain maps from root-object method name to
	// information about that method. The term "obtain"
	// is because these methods obtain an object to
	// call an action on.
	obtain map[string]*obtainer

	// action maps from an object type (as returned by
	// an obtainer method) to the set of methods on an
	// object of that type, with information about each
	// method.
	action map[reflect.Type]map[string]*action
}

// methods returns information on the RPC methods
// implemented by the given type.
func methods(rootType reflect.Type) (*serverMethods, error) {
	typeMutex.RLock()
	methods := methodsByType[rootType]
	typeMutex.RUnlock()
	if methods != nil {
		return methods, nil
	}
	typeMutex.Lock()
	defer typeMutex.Unlock()
	methods = methodsByType[rootType]
	if methods != nil {
		return methods, nil
	}
	methods = &serverMethods{
		obtain: make(map[string]*obtainer),
		action: make(map[reflect.Type]map[string]*action),
	}
	for i := 0; i < rootType.NumMethod(); i++ {
		rootMethod := rootType.Method(i)
		obtain := methodToObtainer(rootMethod)
		if obtain == nil {
			log.Infof("rpc: discarding obtainer method %#v", rootMethod)
			continue
		}
		actions := make(map[string]*action)
		for i := 0; i < obtain.ret.NumMethod(); i++ {
			obtainMethod := obtain.ret.Method(i)
			if act := methodToAction(obtainMethod); act != nil {
				actions[obtainMethod.Name] = act
			} else {
				log.Infof("rpc: discarding action method %#v", obtainMethod)
			}
		}
		if len(actions) > 0 {
			methods.action[obtain.ret] = actions
			methods.obtain[rootMethod.Name] = obtain
		} else {
			log.Infof("rpc: discarding obtainer %v because its result has no methods", rootMethod.Name)
		}
	}
	if len(methods.obtain) == 0 {
		return nil, fmt.Errorf("no RPC methods found on %s", rootType)
	}
	methodsByType[rootType] = methods
	return methods, nil
}

// obtainer holds information on a root-level method.
type obtainer struct {
	// ret holds the type of the object returned by the method.
	ret reflect.Type

	// call invokes the obtainer method. The rcvr parameter must be
	// the same type as the root object. The given id is passed
	// as an argument to the method.
	call func(rcvr reflect.Value, id string) (reflect.Value, error)
}

func methodToObtainer(m reflect.Method) *obtainer {
	if m.PkgPath != "" {
		return nil
	}
	t := m.Type
	if t.NumIn() != 2 ||
		t.NumOut() != 2 ||
		t.In(1) != stringType ||
		t.Out(1) != errorType {
		return nil
	}
	f := func(rcvr reflect.Value, id string) (r reflect.Value, err error) {
		out := rcvr.Method(m.Index).Call([]reflect.Value{reflect.ValueOf(id)})
		if !out[1].IsNil() {
			err = out[1].Interface().(error)
		}
		r = out[0]
		return
	}
	return &obtainer{
		ret:  t.Out(0),
		call: f,
	}
}

// action holds information about a method on an object
// that can be obtained from the root object.
type action struct {
	// arg holds the argument type of the method, or nil
	// if there is no argument.
	arg reflect.Type

	// ret holds the return type of the method, or nil
	// if the method returns no value.
	ret reflect.Type

	// call calls the action method with the given argument
	// on the given receiver value. If the method does
	// not return a value, the returned value will not be valid.
	call func(rcvr, arg reflect.Value) (reflect.Value, error)
}

func methodToAction(m reflect.Method) *action {
	if m.PkgPath != "" {
		return nil
	}
	var p action
	var assemble func(arg reflect.Value) []reflect.Value
	// N.B. The method type has the receiver as its first argument.
	t := m.Type
	switch {
	case t.NumIn() == 1:
		// Method() ...
		assemble = func(arg reflect.Value) []reflect.Value {
			return nil
		}
	case t.NumIn() == 2:
		// Method(T) ...
		p.arg = t.In(1)
		assemble = func(arg reflect.Value) []reflect.Value {
			return []reflect.Value{arg}
		}
	default:
		return nil
	}

	switch {
	case t.NumOut() == 0:
		// Method(...)
		p.call = func(rcvr, arg reflect.Value) (r reflect.Value, err error) {
			rcvr.Method(m.Index).Call(assemble(arg))
			return
		}
	case t.NumOut() == 1 && t.Out(0) == errorType:
		// Method(...) error
		p.call = func(rcvr, arg reflect.Value) (r reflect.Value, err error) {
			out := rcvr.Method(m.Index).Call(assemble(arg))
			if !out[0].IsNil() {
				err = out[0].Interface().(error)
			}
			return
		}
	case t.NumOut() == 1:
		// Method(...) R
		p.ret = t.Out(0)
		p.call = func(rcvr, arg reflect.Value) (reflect.Value, error) {
			out := rcvr.Method(m.Index).Call(assemble(arg))
			return out[0], nil
		}
	case t.NumOut() == 2 && t.Out(1) == errorType:
		// Method(...) (R, error)
		p.ret = t.Out(0)
		p.call = func(rcvr, arg reflect.Value) (r reflect.Value, err error) {
			out := rcvr.Method(m.Index).Call(assemble(arg))
			r = out[0]
			if !out[1].IsNil() {
				err = out[1].Interface().(error)
			}
			return
		}
	default:
		return nil
	}
	if p.arg != nil && p.arg.Kind() != reflect.Struct {
		return nil
	}
	if p.ret != nil && p.ret.Kind() != reflect.Struct {
		return nil
	}
	return &p
}
