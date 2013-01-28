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

var (
	errorType     = reflect.TypeOf((*error)(nil)).Elem()
	interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
	stringType    = reflect.TypeOf("")
)

var errNilDereference = errors.New("field retrieval from nil reference")

type Server struct {
	newRoot func(ctxt interface{}) (reflect.Value, error)
	obtain  map[string]*obtainer
	action  map[reflect.Type]map[string]*action
}

// NewServer returns a new server.  The newRoot value must be a function
// of the form:
//
//     func(ctxt interface{}) (T, error)
//
// where ctxt is a connection-specific value representing the client's
// connection.  For instance, when using Server.Accept, ctxt will be a
// net.Conn.  The value returned by newRoot, the "root" value, is used
// to serve requests for a single client.
// TODO if root value implements io.Closer, call Close.
//
// The server executes each client request by calling a "type" method on
// the root value to obtain an object to act on; then it invokes an
// action method on that object with the request parameters, possibly
// returning some result.
//
// Type methods on the root value are of the form:
//
//      M(id string) (O, error)
//
// where M is an exported name, conventionally naming the object type,
// id is some identifier for the object and O is the type of the
// returned object.
//
// Action methods defined on O may defined in one of the following
// forms, where T and R each represent an arbitrary type other than the
// built-in error type.
//
//	Method()
//	Method() R
//	Method() (R, error)
//	Method() error
//	Method(T)
//	Method(T) R
//	Method(T) (R, error)
//	Method(T) error
//
func NewServer(newRoot interface{}) (*Server, error) {
	rfv := reflect.ValueOf(newRoot)
	rft := rfv.Type()
	if rft.Kind() != reflect.Func ||
		rft.NumIn() != 1 ||
		rft.NumOut() != 2 ||
		rft.In(0) != interfaceType ||
		rft.Out(1) != errorType {
		return nil, fmt.Errorf("newRoot has unexpected type signature %s", rft)
	}
	srv := &Server{
		newRoot: func(ctxt interface{}) (rv reflect.Value, err error) {
			r := rfv.Call([]reflect.Value{reflect.ValueOf(ctxt)})
			rv = r[0]
			if !r[1].IsNil() {
				err = r[1].Interface().(error)
			}
			return
		},
		obtain: make(map[string]*obtainer),
		action: make(map[reflect.Type]map[string]*action),
	}
	rt := rft.Out(0)
	for i := 0; i < rt.NumMethod(); i++ {
		m := rt.Method(i)
		o := srv.methodToObtainer(m)
		if o == nil {
			continue
		}
		actions := make(map[string]*action)
		for i := 0; i < o.ret.NumMethod(); i++ {
			m := o.ret.Method(i)
			if a := srv.methodToAction(m); a != nil {
				actions[m.Name] = a
			}
		}
		if len(actions) > 0 {
			srv.action[o.ret] = actions
			srv.obtain[m.Name] = o
		}
	}
	if len(srv.obtain) == 0 {
		return nil, fmt.Errorf("no RPC methods found on %s", rt)
	}
	return srv, nil
}

type obtainer struct {
	ret  reflect.Type
	call func(rcvr reflect.Value, id string) (reflect.Value, error)
}

func (o *obtainer) String() string {
	return fmt.Sprintf("{id -> %s}", o.ret)
}

func (srv *Server) methodToObtainer(m reflect.Method) *obtainer {
	if m.PkgPath != "" {
		return nil
	}
	t := m.Type
	if t.NumIn() != 1 ||
		t.NumOut() != 2 ||
		t.In(0) != stringType ||
		t.Out(1) != errorType {
		return nil
	}
	f := func(rcvr reflect.Value, id string) (r reflect.Value, err error) {
		out := rcvr.Call([]reflect.Value{rcvr, reflect.ValueOf(id)})
		if !out[1].IsNil() {
			err = out[1].Interface().(error)
		}
		r = out[0]
		return
	}
	return &obtainer{
		t.Out(0),
		f,
	}
}

type action struct {
	arg, ret reflect.Type
	call     func(rcvr, arg reflect.Value) (reflect.Value, error)
}

func (p *action) String() string {
	return fmt.Sprintf("{%s -> %s}", p.arg, p.ret)
}

func (srv *Server) methodToAction(m reflect.Method) *action {
	if m.PkgPath != "" {
		return nil
	}
	var p action
	var assemble func(arg reflect.Value) []reflect.Value
	// N.B. The method type has the receiver as its first argument.
	t := m.Type
	switch {
	case t.NumIn() == 1:
		assemble = func(arg reflect.Value) []reflect.Value {
			return nil
		}
	case t.NumIn() == 2:
		p.arg = t.In(1)
		assemble = func(arg reflect.Value) []reflect.Value {
			return []reflect.Value{arg}
		}
	default:
		return nil
	}

	switch {
	case t.NumOut() == 0:
		p.call = func(rcvr, arg reflect.Value) (r reflect.Value, err error) {
			rcvr.Method(m.Index).Call(assemble(arg))
			return
		}
	case t.NumOut() == 1 && t.Out(0) == errorType:
		p.call = func(rcvr, arg reflect.Value) (r reflect.Value, err error) {
			out := rcvr.Method(m.Index).Call(assemble(arg))
			if !out[0].IsNil() {
				err = out[0].Interface().(error)
			}
			return
		}
	case t.NumOut() == 1:
		p.ret = t.Out(0)
		p.call = func(rcvr, arg reflect.Value) (reflect.Value, error) {
			out := rcvr.Method(m.Index).Call(assemble(arg))
			return out[0], nil
		}
	case t.NumOut() == 2 && t.Out(1) == errorType:
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
	return &p
}
