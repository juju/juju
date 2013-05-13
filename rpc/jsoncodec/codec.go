package jsoncodec

import (
	"encoding/json"
	"io"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/rpc"
)

// JSONConn sends and receives messages to an underlying connection
// in JSON format.
type JSONConn interface {
	// Send sends a message.
	Send(msg interface{}) error
	// Receive receives a message into msg.
	Receive(msg interface{}) error
	Close() error
}

var logRequests = true

// codec implements rpc.Codec for a connection.
type codec struct {
	msgs chan inMsg
	// msg holds the message that's just been read by
	// ReadHeader, so that the body can be read
	// by ReadBody.
	msg   inMsg
	conn  JSONConn
	dying chan struct{}
}

// New returns an rpc codec that uses conn to send and receive
// messages.
func New(conn JSONConn) rpc.Codec {
	c := &codec{
		msgs:  make(chan inMsg),
		conn:  conn,
		dying: make(chan struct{}),
	}
	go c.readRequests()
	return c
}

// inMsg holds an incoming message.  We don't know the type of the
// parameters or response yet, so we delay parsing by storing them
// in a RawMessage.
type inMsg struct {
	RequestId uint64
	Type      string
	Id        string
	Request   string
	Params    json.RawMessage
	Error     string
	ErrorCode string
	Response  json.RawMessage
}

// outMsg holds an outgoing message.
type outMsg struct {
	RequestId uint64
	Type      string      `json:",omitempty"`
	Id        string      `json:",omitempty"`
	Request   string      `json:",omitempty"`
	Params    interface{} `json:",omitempty"`
	Error     string      `json:",omitempty"`
	ErrorCode string      `json:",omitempty"`
	Response  interface{} `json:",omitempty"`
}

func (c *codec) readRequests() {
	defer close(c.msgs)
	var req inMsg
	for {
		var err error
		req = inMsg{} // avoid any potential cross-message contamination.
		if logRequests {
			var m json.RawMessage
			err = c.conn.Receive(&m)
			if err == nil {
				log.Debugf("rpc/wsjson: <- %s", m)
				err = json.Unmarshal(m, &req)
			} else {
				log.Debugf("rpc/wsjson: <- error: %v", err)
			}
		} else {
			err = c.conn.Receive(&req)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Errorf("rpc/wsjson: error receiving request: %v", err)
			break
		}
		c.msgs <- req
	}
}

func (c *codec) Close() error {
	// Possible BUG(rog): this does not cause the underlying
	// connection to be closed - if an RPC client has a huge amount
	// of outstanding requests and is not reading them, we might
	// block writing a message and Close won't unblock it.  This
	// might just conceivably lead to a DOS vulnerability by causing
	// rpc.Conn.Close to block indefinitely.
	close(c.dying)
	return nil
}

func (c *codec) ReadHeader(hdr *rpc.Header) error {
	// We don't read the connection directly here because we want to
	// be able to shut down cleanly without getting spurious errors
	// from closing the connection while we're reading a message.
	// If the codec is closed,
	var ok bool
	select {
	case c.msg, ok = <-c.msgs:
	case <-c.dying:
	}
	if !ok {
		c.conn.Close()
		// Wait for readRequests to see the closed connection and quit.
		for _ = range c.msgs {
		}
		return io.EOF
	}
	hdr.RequestId = c.msg.RequestId
	hdr.Type = c.msg.Type
	hdr.Id = c.msg.Id
	hdr.Request = c.msg.Request
	hdr.Error = c.msg.Error
	hdr.ErrorCode = c.msg.ErrorCode
	return nil
}

func (c *codec) ReadBody(body interface{}, isRequest bool) error {
	if body == nil {
		return nil
	}
	var rawBody json.RawMessage
	if isRequest {
		rawBody = c.msg.Params
	} else {
		rawBody = c.msg.Response
	}
	return json.Unmarshal(rawBody, body)
}

func (c *codec) WriteMessage(hdr *rpc.Header, body interface{}) error {
	r := &outMsg{
		RequestId: hdr.RequestId,

		Error:     hdr.Error,
		ErrorCode: hdr.ErrorCode,
		Response:  body,

		Type:    hdr.Type,
		Id:      hdr.Id,
		Request: hdr.Request,
		Params:  body,
	}
	if logRequests {
		data, err := json.Marshal(r)
		if err != nil {
			log.Debugf("api: -> marshal error: %v", err)
			return err
		}
		log.Debugf("api: -> %s", data)
	}
	return c.conn.Send(r)
}
