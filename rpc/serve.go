package rpc
import (
	"fmt"
	"errors"
	"strings"
	"io"
	"path"
	"reflect"
)

type ServerCodec interface {
	ReadRequestHeader(*Request) error
	ReadRequestBody(interface{}) error
	WriteResponse(*Response, interface{}) error
}

type Request struct {
	Path string
	Seq uint64
}

type Response struct {
	Seq           uint64 // echoes that of the request
	Error         string // error, if any.
	ErrorPath string	// path where the error was encountered.
}

type codecServer struct {
	*Server
	codec ServerCodec
	req Request
	doneReadBody bool
	ctxt reflect.Value
}

func (srv *Server) ServeCodec(codec ServerCodec, ctxt interface{}) error {
	if srv.checkContext != nil {
		if err := srv.checkContext(ctxt); err != nil {
			return err
		}
	}
	csrv := &codecServer {
		Server: srv,
		codec: codec,
		ctxt: reflect.ValueOf(ctxt),
	}
	for {
		csrv.req = Request{}
		err := codec.ReadRequestHeader(&csrv.req)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		csrv.doneReadBody = false
		rv, err := csrv.runRequest()
		if err != nil {
			if !csrv.doneReadBody {
				_ = codec.ReadRequestBody(nil)
			}
			resp := &Response{
				Seq: csrv.req.Seq,
			}
			if e, ok := err.(*pathError); ok {
				resp.Error = e.reason.Error()
				resp.ErrorPath = strings.Join(e.elems, "/")
			} else {
				resp.Error = err.Error()
			}
			if err := codec.WriteResponse(resp, nil); err != nil {
				return err
			}
			continue
		}
		var rvi interface{}
		if rv.IsValid() {
			rvi = rv.Interface()
		}
		if err := codec.WriteResponse(&Response{Seq: csrv.req.Seq}, rvi); err != nil {
			return err
		}
	}
	panic("unreachable")
}

func (csrv *codecServer) readRequestBody(arg interface{}) error {
	csrv.doneReadBody = true
	return csrv.codec.ReadRequestBody(arg)
}

func (csrv *codecServer) runRequest() (reflect.Value, error) {
	elems := strings.FieldsFunc(csrv.req.Path, isSlash)
	relems, err := csrv.lookPath(elems)
	if err != nil {
		return reflect.Value{}, err
	}
	lastElem := &relems[len(relems)-1]
	if lastElem.arg.IsValid() || lastElem.p.arg == nil {
		// If the last element has specified a path argument,
		// or the procedure has no arguments, so
		// discard any other RPC parameters.
		if err := csrv.readRequestBody(nil); err != nil {
			return reflect.Value{}, err
		}
	} else {
		argp := reflect.New(lastElem.p.arg)
		if err := csrv.readRequestBody(argp.Interface()); err != nil {
			return reflect.Value{}, err
		}
		lastElem.arg = argp.Elem()
	}
	// We've verified the path and the arguments; now evaluate the call.
	v := csrv.root
	for i, r := range relems {
		rv, err := r.p.call(v, csrv.ctxt, r.arg)
		if err != nil {
			if i == len(relems)-1 {
				return reflect.Value{}, err
			}
			return reflect.Value{}, &pathError{err, elems[0:i+1]}
		}
		v = rv
	}
	return v, nil
}

func isSlash(r rune) bool {
	return r == '/' 
}

type pathError struct {
	reason error
	elems []string
}

func (e *pathError) Error() string {
	return fmt.Sprintf("error at %q: %v", path.Join(e.elems...), e.reason)
}

type resolvedElem struct {
	name string
	arg reflect.Value
	p *procedure
}

func (srv *Server) Call(path string, ctxt interface{}, arg reflect.Value) (reflect.Value, error) {
	if srv.checkContext != nil {
		if err := srv.checkContext(ctxt); err != nil {
			return reflect.Value{}, err
		}
	}
	elems := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	relems, err := srv.lookPath(elems)
	if err != nil {
		return reflect.Value{}, err
	}
	v := srv.root
	ctxtv := reflect.ValueOf(ctxt)
	for i, r := range relems {
		isLast := i == len(relems)-1
		parg := r.arg
		if isLast && !parg.IsValid() {
			// Note: we discard any argument parameters if
			// a final path argument is specified.
			parg = arg
		}
		rv, err := r.p.call(v, ctxtv, parg)
		if err != nil {
			if isLast {
				return reflect.Value{}, err
			}
			return reflect.Value{}, &pathError{err, elems[0:i+1]}
		}
		v = rv
	}
	return v, nil
}

func (srv *Server) lookPath(elems []string) ([]resolvedElem, error) {
	if len(elems) == 0 {
		return nil, errors.New("empty path")
	}
	relems := make([]resolvedElem, len(elems))
	t := srv.root.Type()
	for i, e := range elems {
		r, err := srv.resolveElem(t, e)
		if err != nil {
			return nil, &pathError{err, elems[0:i+1]}
		}
		t = r.p.ret
		relems[i] = r
	}
	return relems, nil
}

func (srv *Server) resolveElem(t reflect.Type, e string) (r resolvedElem, err error) {
	if t == nil {
		return resolvedElem{}, errors.New("not found0")
	}
	members, ok := srv.types[t]
	if !ok {
		panic(fmt.Errorf("type %s not found", t))
	}
	hyphen := strings.Index(e, "-")
	if hyphen > 0 {
		r.arg = reflect.ValueOf(e[hyphen+1:])
		r.name = e[0:hyphen]
	} else {
		r.name = e
	}
	if p, ok := members[r.name]; ok {
		r.p = p
	} else {
		return resolvedElem{}, errors.New("not found")
	}
	if r.arg.IsValid() {
		if r.p.arg != reflect.TypeOf("") {
			return resolvedElem{}, fmt.Errorf("string argument given for inappropriate method/field: %v", r.p.arg)
		}
	}
	return
}
