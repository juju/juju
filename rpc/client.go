package rpc

import (
	"errors"
	"fmt"
	"io"
	"launchpad.net/juju-core/log"
	"sync"
)

var ErrShutdown = errors.New("connection is shut down")

// A ClientCodec implements writing of RPC requests and reading of RPC
// responses for the client side of an RPC session.  The client calls
// WriteRequest to write a request to the connection and calls
// ReadResponseHeader and ReadResponseBody in pairs to read responses.
// The client calls Close when finished with the connection.
// ReadResponseBody may be called with a nil argument to force the body
// of the response to be read and then discarded.
type ClientCodec interface {
	WriteRequest(*Request, interface{}) error
	ReadResponseHeader(*Response) error
	ReadResponseBody(interface{}) error
	Close() error
}

// Client represents an RPC Client.  There may be multiple outstanding
// Calls associated with a single Client, and a Client may be used by
// multiple goroutines simultaneously.
type Client struct {
	sending sync.Mutex
	codec   ClientCodec
	request Request

	mutex    sync.Mutex // protects the following fields
	reqId    uint64
	pending  map[uint64]*Call
	closing  bool
	shutdown bool
}

// NewClient returns a new Client to handle requests to the set of
// services at the other end of the connection.  The given codec is used
// to encode requests and decode responses.
func NewClientWithCodec(codec ClientCodec) *Client {
	client := &Client{
		codec:   codec,
		pending: make(map[uint64]*Call),
	}
	go client.input()
	return client
}

// Call represents an active RPC.
type Call struct {
	Type     string
	Id       string
	Request  string
	Params   interface{}
	Response interface{}
	Error    error
	Done     chan *Call
}

// ServerError represents an error returned from an RPC server.
type ServerError struct {
	Message string
}

func (e *ServerError) Error() string {
	return "server error: " + e.Message
}

func (client *Client) Close() error {
	client.mutex.Lock()
	if client.shutdown || client.closing {
		client.mutex.Unlock()
		return ErrShutdown
	}
	client.closing = true
	client.mutex.Unlock()
	return client.codec.Close()
}

func (client *Client) send(call *Call) {
	client.sending.Lock()
	defer client.sending.Unlock()

	// Register this call.
	client.mutex.Lock()
	if client.shutdown {
		call.Error = ErrShutdown
		client.mutex.Unlock()
		call.done()
		return
	}
	client.reqId++
	reqId := client.reqId
	client.pending[reqId] = call
	client.mutex.Unlock()

	// Encode and send the request.
	client.request = Request{
		RequestId: reqId,
		Type:      call.Type,
		Id:        call.Id,
		Request:   call.Request,
	}
	if err := client.codec.WriteRequest(&client.request, call.Params); err != nil {
		client.mutex.Lock()
		call = client.pending[reqId]
		delete(client.pending, reqId)
		client.mutex.Unlock()
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (client *Client) readBody(resp interface{}) error {
	err := client.codec.ReadResponseBody(resp)
	if err != nil {
		err = fmt.Errorf("error reading body: %v", err)
	}
	return err
}

func (client *Client) input() {
	var err error
	var response Response
	for err == nil {
		response = Response{}
		err = client.codec.ReadResponseHeader(&response)
		if err != nil {
			if err == io.EOF && !client.closing {
				err = io.ErrUnexpectedEOF
			}
			break
		}
		reqId := response.RequestId
		client.mutex.Lock()
		call := client.pending[reqId]
		delete(client.pending, reqId)
		client.mutex.Unlock()

		switch {
		case call == nil:
			// We've got no pending call. That usually means that
			// WriteRequest partially failed, and call was already
			// removed; response is a server telling us about an
			// error reading request body. We should still attempt
			// to read error body, but there's no one to give it to.
			err = client.readBody(nil)
		case response.Error != "":
			// We've got an error response. Give this to the request;
			// any subsequent requests will get the ReadResponseBody
			// error if there is one.
			call.Error = &ServerError{response.Error}
			err = client.readBody(nil)
			call.done()
		default:
			err = client.readBody(call.Response)
			call.done()
		}
	}
	// Terminate pending calls.
	client.sending.Lock()
	client.mutex.Lock()
	client.shutdown = true
	closing := client.closing
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
	client.pending = nil
	client.mutex.Unlock()
	client.sending.Unlock()
	if err != io.EOF && !closing {
		log.Printf("rpc: client protocol error: %v", err)
	}
}

func (call *Call) done() {
	select {
	case call.Done <- call:
		// ok
	default:
		// We don't want to block here.  It is the caller's responsibility to make
		// sure the channel has enough buffer space. See comment in Go().
		log.Printf("rpc: discarding Call reply due to insufficient Done chan capacity")
	}
}

// Call invokes the named action on the object of the given type with
// the given id.  The returned values will be stored in response, which
// should be a pointer.  If the action fails remotely, the returned
// error will be of type ServerError.
// The params value may be nil if no parameters are provided;
// the response value may be nil if to discard any result value.
func (c *Client) Call(objType, id, action string, params, response interface{}) error {
	call := <-c.Go(objType, id, action, params, response, make(chan *Call, 1)).Done
	return call.Error
}

// Go invokes the request asynchronously.  It returns the Call structure representing
// the invocation.  The done channel will signal when the call is complete by returning
// the same Call object.  If done is nil, Go will allocate a new channel.
// If non-nil, done must be buffered or Go will deliberately panic.
func (c *Client) Go(objType, id, request string, args, response interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10) // buffered.
	} else {
		// If caller passes done != nil, it must arrange that
		// done has enough buffer for the number of simultaneous
		// RPCs that will be using that channel.  If the channel
		// is totally unbuffered, it's best not to run at all.
		if cap(done) == 0 {
			panic("launchpad.net/juju-core/rpc: done channel is unbuffered")
		}
	}
	// Make sure we always send a struct even when the caller
	// provides a nil argment.
	if args == nil {
		args = struct{}{}
	}
	call := &Call{
		Type:     objType,
		Id:       id,
		Request:  request,
		Params:   args,
		Response: response,
		Done:     done,
	}
	c.send(call)
	return call
}
