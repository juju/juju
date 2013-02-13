package rpc

import (
	"fmt"
	"io"
	"launchpad.net/juju-core/log"
	"reflect"
	"sync"
)

// A ServerCodec implements reading of RPC requests and writing of RPC
// responses for the server side of an RPC session.  The server calls
// ReadRequestHeader and ReadRequestBody in pairs to read requests from
// the connection, and it calls WriteResponse to write a response back.
// The params argument to ReadRequestBody will always be of struct type.
// The result argument to WriteResponse will always be a pointer to a struct.
type ServerCodec interface {
	ReadRequestHeader(req *Request) error
	ReadRequestBody(params interface{}) error
	WriteResponse(resp *Response, result interface{}) error
}

// Request is a header written before every RPC call.
type Request struct {
	// RequestId holds the sequence number of the request.
	RequestId uint64

	// Type holds the type of object to act on.
	Type string

	// Id holds the id of the object to act on.
	Id string

	// Request holds the action to invoke on the remote object.
	Request string
}

// Response is a header written before every RPC return.
type Response struct {
	// RequestId echoes that of the request.
	RequestId uint64

	// Error holds the error, if any.
	Error string
}

// codecServer represents an active server instance.
type codecServer struct {
	*Server
	codec ServerCodec

	// pending represents the currently pending requests.
	pending sync.WaitGroup

	// root holds the root value being served.
	root reflect.Value

	// sending guards the write side of the codec.
	sending sync.Mutex
}

// ServeCodec runs the server on a single connection.  ServeCodec
// blocks, serving the connection until the client hangs up.  The caller
// typically invokes ServeCodec in a go statement.  The given
// root value, which must be the same type as that passed to
// NewServer, is used to invoke the RPC requests. If rootValue
// nil, the original root value passed to NewServer will
// be used instead.
//
// ServeCodec will only return when all its outstanding calls have
// completed.
func (srv *Server) ServeCodec(codec ServerCodec, rootValue interface{}) error {
	return srv.serve(reflect.ValueOf(rootValue), codec)
}

func (srv *Server) serve(root reflect.Value, codec ServerCodec) error {
	// TODO(rog) allow concurrent requests.
	if root.Type() != srv.root.Type() {
		panic(fmt.Errorf("rpc: unexpected type of root value; got %s, want %s", root.Type(), srv.root.Type()))
	}
	csrv := &codecServer{
		Server: srv,
		codec:  codec,
		root:   root,
	}
	defer csrv.pending.Wait()
	var req Request
	for {
		req = Request{}
		err := codec.ReadRequestHeader(&req)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		o, a, err := csrv.findRequest(&req)
		if err != nil {
			_ = codec.ReadRequestBody(&struct{}{})
			resp := &Response{
				RequestId: req.RequestId,
			}
			resp.Error = err.Error()
			if err := codec.WriteResponse(resp, struct{}{}); err != nil {
				return err
			}
			continue
		}
		var argp interface{}
		var arg reflect.Value
		if a.arg != nil {
			v := reflect.New(a.arg)
			arg = v.Elem()
			argp = v.Interface()
		} else {
			argp = &struct{}{}
		}
		if err := csrv.codec.ReadRequestBody(argp); err != nil {
			return fmt.Errorf("error reading request body: %v", err)
		}
		csrv.pending.Add(1)
		go csrv.runRequest(req.RequestId, req.Id, o, a, arg)
	}
	panic("unreachable")
}

func (csrv *codecServer) findRequest(req *Request) (*obtainer, *action, error) {
	o := csrv.obtain[req.Type]
	if o == nil {
		return nil, nil, fmt.Errorf("unknown object type %q", req.Type)
	}
	a := csrv.action[o.ret][req.Request]
	if a == nil {
		return nil, nil, fmt.Errorf("no such action %q on %s", req.Request, req.Type)
	}
	return o, a, nil
}

// runRequest runs the given request and sends the reply.
func (csrv *codecServer) runRequest(reqId uint64, objId string, o *obtainer, a *action, arg reflect.Value) {
	defer csrv.pending.Done()
	rv, err := csrv.runRequest0(reqId, objId, o, a, arg)
	csrv.sending.Lock()
	defer csrv.sending.Unlock()
	var rvi interface{}
	resp := &Response{
		RequestId: reqId,
	}
	if err != nil {
		resp.Error = err.Error()
		rvi = struct{}{}
	} else if rv.IsValid() {
		rvi = rv.Interface()
	} else {
		rvi = struct{}{}
	}
	if err := csrv.codec.WriteResponse(resp, rvi); err != nil {
		log.Printf("rpc: error writing response %#v: %v", rvi, err)
	}
}

func (csrv *codecServer) runRequest0(reqId uint64, objId string, o *obtainer, a *action, arg reflect.Value) (reflect.Value, error) {
	obj, err := o.call(csrv.root, objId)
	if err != nil {
		return reflect.Value{}, err
	}
	return a.call(obj, arg)
}
