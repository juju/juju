// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpcreflect

import (
	"errors"
	"reflect"
	"sort"
	"sync"
)

var (
	errorType  = reflect.TypeOf((*error)(nil)).Elem()
	stringType = reflect.TypeOf("")
)

var (
	typeMutex     sync.RWMutex
	typesByGoType = make(map[reflect.Type]*Type)

	objTypeMutex     sync.RWMutex
	objTypesByGoType = make(map[reflect.Type]*ObjType)
)

var ErrMethodNotFound = errors.New("no such method")

// Type holds information about a type that implements RPC server methods,
// a root-level RPC type.
type Type struct {
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
func (r *Type) MethodNames() []string {
	names := make([]string, 0, len(r.method))
	for name := range r.method {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Method returns information on the method with the given name,
// or the zero Method and ErrMethodNotFound if there is no such method
// (the only possible error).
func (r *Type) Method(name string) (RootMethod, error) {
	m, ok := r.method[name]
	if !ok {
		return RootMethod{}, ErrMethodNotFound
	}
	return *m, nil
}

func (r *Type) DiscardedMethods() []string {
	return append([]string(nil), r.discarded...)
}

// RootMethod holds information on a root-level method.
type RootMethod struct {
	// Call invokes the method. The rcvr parameter must be
	// the same type as the root object. The given id is passed
	// as an argument to the method.
	Call func(rcvr reflect.Value, id string) (reflect.Value, error)

	// Methods holds RPC object-method information about
	// objects returned from the above call.
	ObjType *ObjType
}

// TypeOf returns information on all root-level RPC methods
// implemented by an object of the given Go type.
// If goType is nil, it returns nil.
func TypeOf(goType reflect.Type) *Type {
	if goType == nil {
		return nil
	}
	typeMutex.RLock()
	t := typesByGoType[goType]
	typeMutex.RUnlock()
	if t != nil {
		return t
	}
	typeMutex.Lock()
	defer typeMutex.Unlock()
	t = typesByGoType[goType]
	if t != nil {
		return t
	}
	t = typeOf(goType)
	typesByGoType[goType] = t
	return t
}

// typeOf is like TypeOf but without the cache - it
// always allocates. Called with rootTypeMutex locked.
func typeOf(goType reflect.Type) *Type {
	rm := &Type{
		method: make(map[string]*RootMethod),
	}
	for i := 0; i < goType.NumMethod(); i++ {
		m := goType.Method(i)
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
			// Workaround LP 1251076.
			// gccgo appears to get confused and thinks that out[1] is not nil when
			// in fact it is an interface value of type error and value nil.
			// This workaround solves the problem by leaving error as nil if in fact
			// it was nil and causes no harm for gc because the predicates above
			// assert that if out[1] is not nil, then it is an error type.
			err, _ = out[1].Interface().(error)
		}
		r = out[0]
		return
	}
	return &RootMethod{
		Call:    f,
		ObjType: ObjTypeOf(t.Out(0)),
	}
}

// ObjType holds information on RPC methods implemented on
// an RPC object.
type ObjType struct {
	goType    reflect.Type
	method    map[string]*ObjMethod
	discarded []string
}

func (t *ObjType) GoType() reflect.Type {
	return t.goType
}

// Method returns information on the method with the given name,
// or the zero Method and ErrMethodNotFound if there is no such method
// (the only possible error).
func (t *ObjType) Method(name string) (ObjMethod, error) {
	m, ok := t.method[name]
	if !ok {
		return ObjMethod{}, ErrMethodNotFound
	}
	return *m, nil
}

// DiscardedMethods returns the names of all methods that cannot
// implement RPC calls because their type signature is inappropriate.
func (t *ObjType) DiscardedMethods() []string {
	return append([]string(nil), t.discarded...)
}

// MethodNames returns the names of all the RPC methods
// defined on the type.
func (t *ObjType) MethodNames() []string {
	names := make([]string, 0, len(t.method))
	for name := range t.method {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ObjMethod holds information about an RPC method on an
// object returned by a root-level method.
type ObjMethod struct {
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

// ObjTypeOf returns information on all RPC methods
// implemented by an object of the given Go type,
// as returned from a root-level method.
// If objType is nil, it returns nil.
func ObjTypeOf(objType reflect.Type) *ObjType {
	if objType == nil {
		return nil
	}
	objTypeMutex.RLock()
	methods := objTypesByGoType[objType]
	objTypeMutex.RUnlock()
	if methods != nil {
		return methods
	}
	objTypeMutex.Lock()
	defer objTypeMutex.Unlock()
	methods = objTypesByGoType[objType]
	if methods != nil {
		return methods
	}
	methods = objTypeOf(objType)
	objTypesByGoType[objType] = methods
	return methods
}

// objTypeOf is like ObjTypeOf but without the cache.
// Called with objTypeMutex locked.
func objTypeOf(goType reflect.Type) *ObjType {
	objType := &ObjType{
		method: make(map[string]*ObjMethod),
		goType: goType,
	}
	for i := 0; i < goType.NumMethod(); i++ {
		m := goType.Method(i)
		if m.PkgPath != "" {
			continue
		}
		if objm := newMethod(m, goType.Kind()); objm != nil {
			objType.method[m.Name] = objm
		} else {
			objType.discarded = append(objType.discarded, m.Name)
		}
	}
	return objType
}

func newMethod(m reflect.Method, receiverKind reflect.Kind) *ObjMethod {
	if m.PkgPath != "" {
		return nil
	}
	var p ObjMethod
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
