// The jsoncodec package provides a JSON codec for the rpc package.
package jsoncodec

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/rpc"
)

var logger = loggo.GetLogger("juju.rpc.jsoncodec")

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
			logger.Tracef("<- %s", m)
			err = json.Unmarshal(m, &c.msg)
		} else {
			logger.Tracef("<- error: %v (closing %v)", err, c.isClosing())
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
	hdr.Request = rpc.Request{
		Type:   c.msg.Type,
		Id:     c.msg.Id,
		Action: c.msg.Request,
	}
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

// DumpRequest returns JSON-formatted data representing
// the RPC message with the given header and body,
// as it would be written by Codec.WriteMessage.
// If the body cannot be marshalled as JSON, the data
// will hold a JSON string describing the error.
func DumpRequest(hdr *rpc.Header, body interface{}) []byte {
	var m outMsg
	m.init(hdr, body)
	data, err := json.Marshal(&m)
	if err != nil {
		return []byte(fmt.Sprintf("%q", "marshal error: "+err.Error()))
	}
	return data
}

func (c *Codec) WriteMessage(hdr *rpc.Header, body interface{}) error {
	var m outMsg
	m.init(hdr, body)
	if c.isLogging() {
		data, err := json.Marshal(&m)
		if err != nil {
			logger.Tracef("-> marshal error: %v", err)
			return err
		}
		logger.Tracef("-> %s", data)
	}
	return c.conn.Send(&m)
}

// init fills out the receiving outMsg with information from the given
// header and body.
func (m *outMsg) init(hdr *rpc.Header, body interface{}) {
	m.RequestId = hdr.RequestId
	m.Type = hdr.Request.Type
	m.Id = hdr.Request.Id
	m.Request = hdr.Request.Action
	m.Error = hdr.Error
	m.ErrorCode = hdr.ErrorCode
	if hdr.IsRequest() {
		m.Params = body
	} else {
		m.Response = body
	}
}
