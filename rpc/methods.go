// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"launchpad.net/juju-core/log"
	"reflect"
	"sort"
	"sync"
)

var (
	errorType  = reflect.TypeOf((*error)(nil)).Elem()
	stringType = reflect.TypeOf("")
)

var (
	rootTypeMutex     sync.RWMutex
	rootMethodsByType = make(map[reflect.Type]*RootMethods)

	objTypeMutex     sync.RWMutex
	objMethodsByType = make(map[reflect.Type]*Methods)
)

// RootMethods holds information about a type that
// implements RPC server methods.
type RootMethods struct {
	// root holds the root type.
	root reflect.Type

	// method maps from root-object method name to
	// information about that method. The term "obtain"
	// is because these methods obtain an object to
	// call an action on.
	method map[string]*RootMethod

	// discarded holds names of all discarded methods.
	discarded []string
}

// MethodNames returns the names of all the root object
// methods on the receiving object.
func (r *RootMethods) MethodNames() []string {
	var names []string
	for name := range r.method {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Method returns information on the method with the given name,
// or the zero Method and false if there is no such method.
func (r *RootMethods) Method(name string) (RootMethod, bool) {
	m, ok := r.method[name]
	if !ok {
		return RootMethod{}, false
	}
	return *m, true
}

func (r *RootMethods) DiscardedMethods() []string {
	return append([]string(nil), r.discarded...)
}

// RootMethod holds information on a root-level method.
type RootMethod struct {
	// Call invokes the method. The rcvr parameter must be
	// the same type as the root object. The given id is passed
	// as an argument to the method.
	Call func(rcvr reflect.Value, id string) (reflect.Value, error)

	// Type holds the type of value that will be returned
	// by the method.
	Type reflect.Type

	// Methods holds RPC object-method information about
	// objects of the above typr
	Methods *Methods
}

// RootInfo returns information on all the rpc "type" methods
// implemented by an object of the given type.
func RootInfo(rootType reflect.Type) *RootMethods {
	rootTypeMutex.RLock()
	methods := rootMethodsByType[rootType]
	rootTypeMutex.RUnlock()
	if methods != nil {
		return methods
	}
	rootTypeMutex.Lock()
	defer rootTypeMutex.Unlock()
	methods = rootMethodsByType[rootType]
	if methods != nil {
		return methods
	}
	methods = rootInfo(rootType)
	rootMethodsByType[rootType] = methods
	return methods
}

// rootInfo is like RootInfo but without the cache - it
// always allocates. Called with rootTypeMutex locked.
func rootInfo(rootType reflect.Type) *RootMethods {
	rm := &RootMethods{
		method: make(map[string]*RootMethod),
	}
	for i := 0; i < rootType.NumMethod(); i++ {
		m := rootType.Method(i)
		if m.PkgPath != "" || isKillMethod(m) {
			// The Kill method gets a special exception because
			// it fulfils the Killer interface which we're expecting,
			// so it's not really discarded as such.
			continue
		}
		if o := newRootMethod(m); o != nil {
			rm.method[m.Name] = o
		} else {
			rm.discarded = append(rm.discarded, m.Name)
		}
	}
	return rm
}

func isKillMethod(m reflect.Method) bool {
	return m.Name == "Kill" && m.Type.NumIn() == 1 && m.Type.NumOut() == 0
}

func newRootMethod(m reflect.Method) *RootMethod {
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
	_type := t.Out(0)
	return &RootMethod{
		Type:    _type,
		Call:    f,
		Methods: ObjectInfo(_type),
	}
}

// Methods holds information on RPC methods implemented on
// an RPC object.
type Methods struct {
	method    map[string]*Method
	discarded []string
}

// Method returns information on the method with the given name,
// or the zero Method and false if there is no such method.
func (ms *Methods) Method(name string) (method Method, ok bool) {
	m, ok := ms.method[name]
	if !ok {
		return Method{}, false
	}
	return *m, true
}

// DiscardedMethods returns the names of all methods that cannot
// implement RPC calls because their type signature is inappropriate.
func (ms *Methods) DiscardedMethods() []string {
	return append([]string(nil), ms.discarded...)
}

// MethodNames returns the names of all the RPC methods
// defined on the object.
func (m *Methods) MethodNames() []string {
	var names []string
	for name := range m.method {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Method holds information about an RPC method on an
// object returned by a root-level method.
type Method struct {
	// Params holds the argument type of the method, or nil
	// if there is no argument.
	Params reflect.Type

	// Result holds the return type of the method, or nil
	// if the method returns no value.
	Result reflect.Type

	// Call calls the method with the given argument
	// on the given receiver value. If the method does
	// not return a value, the returned value will not be valid.
	Call func(rcvr, arg reflect.Value) (reflect.Value, error)
}

func ObjectInfo(objType reflect.Type) *Methods {
	objTypeMutex.RLock()
	methods := objMethodsByType[objType]
	objTypeMutex.RUnlock()
	if methods != nil {
		return methods
	}
	objTypeMutex.Lock()
	defer objTypeMutex.Unlock()
	methods = objMethodsByType[objType]
	if methods != nil {
		return methods
	}
	methods = objectInfo(objType)
	objMethodsByType[objType] = methods
	return methods
}

// objectInfo is like ObjectInfo but without the cache.
// Called with objTypeMutex locked.
func objectInfo(objType reflect.Type) *Methods {
	objMethods := &Methods{
		method: make(map[string]*Method),
	}
	for i := 0; i < objType.NumMethod(); i++ {
		m := objType.Method(i)
		if m.PkgPath != "" {
			continue
		}
		log.Infof("considering method %#v\n", m)
		if objm := newMethod(m, objType.Kind()); objm != nil {
			objMethods.method[m.Name] = objm
		} else {
			objMethods.discarded = append(objMethods.discarded, m.Name)
		}
	}
	return objMethods
}

func newMethod(m reflect.Method, receiverKind reflect.Kind) *Method {
	if m.PkgPath != "" {
		return nil
	}
	var p Method
	var assemble func(arg reflect.Value) []reflect.Value
	// N.B. The method type has the receiver as its first argument
	// unless the receiver is an interface.
	receiverArgCount := 1
	if receiverKind == reflect.Interface {
		receiverArgCount = 0
	}
	t := m.Type
	switch {
	case t.NumIn() == 0+receiverArgCount:
		// Method() ...
		assemble = func(arg reflect.Value) []reflect.Value {
			return nil
		}
	case t.NumIn() == 1+receiverArgCount:
		// Method(T) ...
		p.Params = t.In(receiverArgCount)
		assemble = func(arg reflect.Value) []reflect.Value {
			return []reflect.Value{arg}
		}
	default:
		return nil
	}

	switch {
	case t.NumOut() == 0:
		// Method(...)
		p.Call = func(rcvr, arg reflect.Value) (r reflect.Value, err error) {
			rcvr.Method(m.Index).Call(assemble(arg))
			return
		}
	case t.NumOut() == 1 && t.Out(0) == errorType:
		// Method(...) error
		p.Call = func(rcvr, arg reflect.Value) (r reflect.Value, err error) {
			out := rcvr.Method(m.Index).Call(assemble(arg))
			if !out[0].IsNil() {
				err = out[0].Interface().(error)
			}
			return
		}
	case t.NumOut() == 1:
		// Method(...) R
		p.Result = t.Out(0)
		p.Call = func(rcvr, arg reflect.Value) (reflect.Value, error) {
			out := rcvr.Method(m.Index).Call(assemble(arg))
			return out[0], nil
		}
	case t.NumOut() == 2 && t.Out(1) == errorType:
		// Method(...) (R, error)
		p.Result = t.Out(0)
		p.Call = func(rcvr, arg reflect.Value) (r reflect.Value, err error) {
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
	// The parameters and return value must be of struct type.
	if p.Params != nil && p.Params.Kind() != reflect.Struct {
		return nil
	}
	if p.Result != nil && p.Result.Kind() != reflect.Struct {
		return nil
	}
	return &p
}
