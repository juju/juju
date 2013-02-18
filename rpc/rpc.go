package rpc

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/log"
	"reflect"
)

var (
	errorType     = reflect.TypeOf((*error)(nil)).Elem()
	interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
	stringType    = reflect.TypeOf("")
)

var errNilDereference = errors.New("field retrieval from nil reference")

// Server represents an RPC server.
type Server struct {
	// root holds the default root value.
	root reflect.Value

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

	// transformErrors is used to process all errors sent to
	// the client.
	transformErrors func(error) error
}

// NewServer returns a new server that will serve requests
// by invoking methods on values of the same type as rootValue.
// Actual values of rootValue may be provided for
// each client connection (see ServeCodec and Accept),
// or rootValue may be used directly if no such values are
// provided.
//
// The server executes each client request by calling a "type" method on
// a root value to obtain an object to act on; then it invokes an
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
// Request methods defined on O may defined in one of the following
// forms, where T and R must be struct types.
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
// If transformErrors is non-nil, it will be called on all returned
// non-nil errors, for example to transform the errors into ServerErrors
// with specified codes.  There will be a panic if transformErrors
// returns nil.
func NewServer(rootValue interface{}, transformErrors func(error) error) (*Server, error) {
	if transformErrors == nil {
		transformErrors = func(e error) error { return e }
	}
	srv := &Server{
		root:            reflect.ValueOf(rootValue),
		obtain:          make(map[string]*obtainer),
		action:          make(map[reflect.Type]map[string]*action),
		transformErrors: transformErrors,
	}
	rt := srv.root.Type()
	for i := 0; i < rt.NumMethod(); i++ {
		m := rt.Method(i)
		o := srv.methodToObtainer(m)
		if o == nil {
			log.Printf("rpc: discarding obtainer method %#v", m)
			continue
		}
		actions := make(map[string]*action)
		for i := 0; i < o.ret.NumMethod(); i++ {
			m := o.ret.Method(i)
			if a := srv.methodToAction(m); a != nil {
				actions[m.Name] = a
			} else {
				log.Printf("rpc: discarding action method %#v", m)
			}
		}
		if len(actions) > 0 {
			srv.action[o.ret] = actions
			srv.obtain[m.Name] = o
		} else {
			log.Printf("rpc: discarding obtainer %v because its result has no methods", m.Name)
		}
	}
	if len(srv.obtain) == 0 {
		return nil, fmt.Errorf("no RPC methods found on %s", rt)
	}
	return srv, nil
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

func (o *obtainer) String() string {
	return fmt.Sprintf("{id -> %s}", o.ret)
}

func (srv *Server) methodToObtainer(m reflect.Method) *obtainer {
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
		t.Out(0),
		f,
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

func (p *action) String() string {
	return fmt.Sprintf("{%v -> %v}", p.arg, p.ret)
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
