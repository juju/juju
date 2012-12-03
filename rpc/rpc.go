package rpc
import (
	"fmt"
	"errors"
	"strings"
	"path"
	"reflect"
)

/*
Things to think about:

can we provide some way of distinguishing GET from POST methods?
*/

var errorType = reflect.TypeOf((*error)(nil)).Elem()
var errNilDereference = errors.New("field retrieval from nil reference")

type Server struct {
	root reflect.Value
	ctxtType reflect.Type
	// We store the member names for each type,
	// each one holding a function that can get the
	// field or method from its parent value.
	types map[reflect.Type] map[string] *procedure
}

// NewServer returns a new server that serves RPC requests by querying
// the given root value.  The request path specifies an object to act
// on.  The first element acts on the root value; each subsequent
// element acts on the value returned by the previous.  The value
// returned by the final element of the path is returned as the result
// of the RPC.  If any element in the path returns an error, evaluation
// stops there.
//
// A element of the path can specify the name of exported field or
// method. To be considered, a method must be defined
// in one of the following forms, where T and R represent
// arbitrary types (except the built-in error type):
//
//     Method() R
//     Method() (R, error)
//	Method(T) R
//	Method(T) (R, error)
//	Method()
//	Method() error
//	Method(T) error
//
// For any element in the path except the final one, the latter three
// forms may not be used, to ensure there is something for the next
// element to operate on; also in this case, the argument type T must be
// of type string - the path element must contain a hyphen (-) character
// and the argument to the method is supplied from any characters after
// that.
//
// For the last element in the path, the method argument will be filled
// in from the parameters passed to the RPC; the R result will be
// returned to the RPC caller.
//
// If the root value implements the method CheckContext, it will be
// called for any new connection before any other method, with the
// argument passed to ServeConn.  In this case, methods with two
// arguments are also considered - the first argument must be the same
// type as CheckContext's argument, and it will likewise be passed the
// context value given to ServerConn.
//
func NewServer(root interface{}) (*Server, error) {
	srv := &Server{
		root: reflect.ValueOf(root),
		types: make(map[reflect.Type] map[string] *procedure),
	}
	t := reflect.TypeOf(root)
	if m, ok := t.MethodByName("CheckContext"); ok {
		if m.Type.NumIn() != 2 ||
			m.Type.NumOut() != 1 ||
			m.Type.Out(0) != errorType {
			return nil, fmt.Errorf("CheckContext has unexpected type %v", m.Type)
		}
		srv.ctxtType = m.Type.In(1)
	}
	srv.buildTypes(t)
	return srv, nil
}

type procedure struct {
	arg, ret reflect.Type
	call func(rcvr, ctxt, arg reflect.Value) (reflect.Value, error)
}

func (srv *Server) buildTypes(t reflect.Type) {
	if _, ok := srv.types[t]; ok {
		return
	}
	members := make(map[string] *procedure)
	// Add first to guard against infinite recursion.
	srv.types[t] = members
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if p := srv.methodToProcedure(m); p != nil {
			members[m.Name] = p
		}
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
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
	// N.B. The method type includes receiver as first argument.
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
	case t.NumIn() == 3:
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
				err = out[0].Interface().(error)
			}
			return
		}
	default:
		return nil
	}
	return &p
}

type ServerCodec interface {
	// Auth is called once only on a given server;
	// it fetches the authentication data into req.
	Auth(req interface{}) error
	ReadRequestHeader(*Request) error
	ReadRequestBody(interface{}) error
	WriteResponse(*Response, interface{}) error
	Close() error
}

type Request struct {
	Path string
	Seq uint64
}

type Response struct {
    ServiceMethod string // echoes that of the Request
    Seq           uint64 // echoes that of the request
    Error         string // error, if any.
}

func (*Server) ServeConn(codec ServerCodec, ctxt interface{}) {
	
}

type pathError struct {
	reason string
	elems []string
}

func (e *pathError) Error() string {
	return fmt.Sprintf("error at %q: %v", path.Join(e.elems...), e.reason)
}

func (srv *Server) Call(path string, ctxt interface{}, arg reflect.Value) (reflect.Value, error) {
	elems := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	v := srv.root
	for i, e := range elems {
		members, ok := srv.types[v.Type()]
		if !ok {
			panic(fmt.Errorf("type %T not found", v))
		}
		hyphen := strings.Index(e, "-")
		var pathArg string
		if hyphen > 0 {
			pathArg = e[hyphen+1:]
			e = e[0:hyphen]
		}
		soFar := elems[0:i+1]
		p, ok := members[e]
		if !ok {
			return reflect.Value{}, &pathError{"not found", soFar}
		}
		isLast := i == len(elems)-1
		if p.ret == nil && !isLast {
			return reflect.Value{}, &pathError{"extra path elements", soFar}
		}
		var parg reflect.Value
		if hyphen > 0 {
			if p.arg != nil || p.arg != reflect.TypeOf("") {
				return reflect.Value{}, &pathError{"string argument given for inappropriate method/field", soFar}
			}
			if isLast && arg.IsValid() {
				return reflect.Value{}, &pathError{"argument clash (path string vs arg)", soFar}
			}
			parg = reflect.ValueOf(pathArg)
		} else if isLast {
			parg = arg
		}
		r, err := p.call(v, reflect.ValueOf(ctxt), parg)
		if err != nil {
			if isLast {
				return reflect.Value{}, err
			}
			return reflect.Value{}, &pathError{err.Error(), soFar}
		}
		v = r
	}
	return v, nil
}
