// The jsoncodec package provides a JSON codec for the rpc package.
package jsoncodec

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

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

// Codec implements rpc.Codec for a connection.
type Codec struct {
	// msg holds the message that's just been read by ReadHeader, so
	// that the body can be read by ReadBody.
	msg         inMsg
	conn        JSONConn
	logMessages int32
	mu          sync.Mutex
	closing     bool
}

// New returns an rpc codec that uses conn to send and receive
// messages.
func New(conn JSONConn) *Codec {
	return &Codec{
		conn: conn,
	}
}

// SetLogging sets whether messages will be logged
// by the codec.
func (c *Codec) SetLogging(on bool) {
	val := int32(0)
	if on {
		val = 1
	}
	atomic.StoreInt32(&c.logMessages, val)
}

func (c *Codec) isLogging() bool {
	return atomic.LoadInt32(&c.logMessages) != 0
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

func (c *Codec) Close() error {
	c.mu.Lock()
	c.closing = true
	c.mu.Unlock()
	return c.conn.Close()
}

func (c *Codec) isClosing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closing
}

func (c *Codec) ReadHeader(hdr *rpc.Header) error {
	c.msg = inMsg{} // avoid any potential cross-message contamination.
	var err error
	if c.isLogging() {
		var m json.RawMessage
		err = c.conn.Receive(&m)
		if err == nil {
			log.Debugf("rpc/jsoncodec: <- %s", m)
			err = json.Unmarshal(m, &c.msg)
		} else {
			log.Debugf("rpc/jsoncodec: <- error: %v (closing %v)", err, c.isClosing())
		}
	} else {
		err = c.conn.Receive(&c.msg)
	}
	if err != nil {
		// If we've closed the connection, we may get a spurious error,
		// so ignore it.
		if c.isClosing() || err == io.EOF {
			return io.EOF
		}
		return fmt.Errorf("error receiving message: %v", err)
	}
	hdr.RequestId = c.msg.RequestId
	hdr.Type = c.msg.Type
	hdr.Id = c.msg.Id
	hdr.Request = c.msg.Request
	hdr.Error = c.msg.Error
	hdr.ErrorCode = c.msg.ErrorCode
	return nil
}

func (c *Codec) ReadBody(body interface{}, isRequest bool) error {
	if body == nil {
		return nil
	}
	var rawBody json.RawMessage
	if isRequest {
		rawBody = c.msg.Params
	} else {
		rawBody = c.msg.Response
	}
	if len(rawBody) == 0 {
		// If the response or params are omitted, it's
		// equivalent to an empty object.
		return nil
	}
	return json.Unmarshal(rawBody, body)
}

func (c *Codec) WriteMessage(hdr *rpc.Header, body interface{}) error {
	r := &outMsg{
		RequestId: hdr.RequestId,

		Type:    hdr.Type,
		Id:      hdr.Id,
		Request: hdr.Request,

		Error:     hdr.Error,
		ErrorCode: hdr.ErrorCode,
	}
	if hdr.IsRequest() {
		r.Params = body
	} else {
		r.Response = body
	}
	if c.isLogging() {
		data, err := json.Marshal(r)
		if err != nil {
			log.Debugf("rpc/jsoncodec: -> marshal error: %v", err)
			return err
		}
		log.Debugf("rpc/jsoncodec: -> %s", data)
	}
	return c.conn.Send(r)
}
