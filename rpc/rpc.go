package rpc

import (
	"errors"
	"fmt"
	"reflect"
)

/*
Things to think about:

can we provide some way of distinguishing GET from POST methods?
*/

var errorType = reflect.TypeOf((*error)(nil)).Elem()
var errNilDereference = errors.New("field retrieval from nil reference")

type Server struct {
	root         reflect.Value
	checkContext func(ctxt interface{}) error
	ctxtType     reflect.Type
	// We store the member names for each type,
	// each one holding a function that can get the
	// field or method from its parent value.
	types map[reflect.Type]map[string]*procedure
}

// NewServer returns a new server that serves RPC requests by querying
// the given root value.  The request path specifies an object to act
// on.  The first element acts on the root value; each subsequent
// element acts on the value returned by the previous element.  The
// value returned by the final element of the path is returned as the
// result of the RPC.  If any element in the path returns an error,
// evaluation stops there.
//
// An element of the path specifies the name of exported field or
// method. To be considered, a method must be defined
// in one of the following forms, where T and R represent
// an arbitrary type other than the built-in error type:
//
//	Method() R
//	Method() (R, error)
//	Method(T) R
//	Method(T) (R, error)
//	Method()
//	Method() error
//	Method(T) error
//
// If a path element contains a hyphen (-) character, the method's
// argument type T must be string, and it will be supplied from any
// characters after the hyphen.
//
// The last element in the path is treated specially.  Its method
// argument argument will be filled in from the parameters passed to the
// RPC and the R result will be returned to the RPC caller.
//
// If the root value implements the method CheckContext, it will be
// called for any new connection before any other method, with the
// argument passed to ServeConn.  In this case, any method may have a
// first argument of this type and it will likewise be passed the
// context value given to ServeConn.
//
func NewServer(root interface{}) (*Server, error) {
	srv := &Server{
		root:  reflect.ValueOf(root),
		types: make(map[reflect.Type]map[string]*procedure),
	}
	t := srv.root.Type()
	if m, ok := t.MethodByName("CheckContext"); ok {
		if m.Type.NumIn() != 2 ||
			m.Type.NumOut() != 1 ||
			m.Type.Out(0) != errorType {
			return nil, fmt.Errorf("CheckContext has unexpected type %v", m.Type)
		}
		srv.ctxtType = m.Type.In(1)
		srv.checkContext = func(ctxt interface{}) error {
			r := srv.root.Method(m.Index).Call([]reflect.Value{
				reflect.ValueOf(ctxt),
			})
			if e := r[0].Interface(); e != nil {
				return e.(error)
			}
			return nil
		}
	}
	srv.buildTypes(t)
	return srv, nil
}

type procedure struct {
	arg, ret reflect.Type
	call     func(rcvr, ctxt, arg reflect.Value) (reflect.Value, error)
}

func (p *procedure) String() string {
	return fmt.Sprintf("{%s -> %s}", p.arg, p.ret)
}

func (srv *Server) buildTypes(t reflect.Type) {
	// log.Printf("buildTypes %s, %d methods", t, t.NumMethod())
	if _, ok := srv.types[t]; ok {
		return
	}
	members := make(map[string]*procedure)
	// Add first to guard against infinite recursion.
	srv.types[t] = members
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if p := srv.methodToProcedure(m); p != nil {
			members[m.Name] = p
		}
	}
	st := t
	if st.Kind() == reflect.Ptr {
		st = st.Elem()
	}
	if st.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		// TODO if t is addressable, use pointer to field so that we get pointer methods?
		if p := fieldToProcedure(t, f, i); p != nil {
			members[f.Name] = p
		}
	}
	for _, m := range members {
		if m.ret != nil {
			srv.buildTypes(m.ret)
		}
	}
}

func fieldToProcedure(rcvr reflect.Type, f reflect.StructField, index int) *procedure {
	if f.PkgPath != "" {
		return nil
	}
	var p procedure
	p.ret = f.Type
	if rcvr.Kind() == reflect.Ptr {
		p.call = func(rcvr, ctxt, arg reflect.Value) (r reflect.Value, err error) {
			if rcvr.IsNil() {
				err = errNilDereference
				return
			}
			rcvr = rcvr.Elem()
			return rcvr.Field(index), nil
		}
	} else {
		p.call = func(rcvr, ctxt, arg reflect.Value) (r reflect.Value, err error) {
			return rcvr.Field(index), nil
		}
	}
	return &p
}

func (srv *Server) methodToProcedure(m reflect.Method) *procedure {
	if m.PkgPath != "" || m.Name == "CheckContext" {
		return nil
	}
	var p procedure
	var assemble func(ctxt, arg reflect.Value) []reflect.Value
	// N.B. The method type has the receiver as its first argument.
	t := m.Type
	switch {
	case t.NumIn() == 1:
		assemble = func(ctxt, arg reflect.Value) []reflect.Value {
			return nil
		}
	case t.NumIn() == 2 && t.In(1) == srv.ctxtType:
		assemble = func(ctxt, arg reflect.Value) []reflect.Value {
			return []reflect.Value{ctxt}
		}
	case t.NumIn() == 2:
		p.arg = t.In(1)
		assemble = func(ctxt, arg reflect.Value) []reflect.Value {
			return []reflect.Value{arg}
		}
	case t.NumIn() == 3 && t.In(1) == srv.ctxtType:
		p.arg = t.In(2)
		assemble = func(ctxt, arg reflect.Value) []reflect.Value {
			return []reflect.Value{ctxt, arg}
		}
	default:
		return nil
	}

	switch {
	case t.NumOut() == 0:
		p.call = func(rcvr, ctxt, arg reflect.Value) (r reflect.Value, err error) {
			rcvr.Method(m.Index).Call(assemble(ctxt, arg))
			return
		}
	case t.NumOut() == 1 && t.Out(0) == errorType:
		p.call = func(rcvr, ctxt, arg reflect.Value) (r reflect.Value, err error) {
			out := rcvr.Method(m.Index).Call(assemble(ctxt, arg))
			if !out[0].IsNil() {
				err = out[0].Interface().(error)
			}
			return
		}
	case t.NumOut() == 1:
		p.ret = t.Out(0)
		p.call = func(rcvr, ctxt, arg reflect.Value) (reflect.Value, error) {
			out := rcvr.Method(m.Index).Call(assemble(ctxt, arg))
			return out[0], nil
		}
	case t.NumOut() == 2 && t.Out(1) == errorType:
		p.ret = t.Out(0)
		p.call = func(rcvr, ctxt, arg reflect.Value) (r reflect.Value, err error) {
			out := rcvr.Method(m.Index).Call(assemble(ctxt, arg))
			r = out[0]
			if !out[1].IsNil() {
				err = out[1].Interface().(error)
			}
			return
		}
	default:
		return nil
	}
	return &p
}
