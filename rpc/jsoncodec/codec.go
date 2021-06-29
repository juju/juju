// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The jsoncodec package provides a JSON codec for the rpc package.
package jsoncodec

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/rpc"
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
	msg         inMsgV1
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

// inMsg holds an incoming message.  We don't know the type of the
// parameters or response yet, so we delay parsing by storing them
// in a RawMessage.
type inMsgV0 struct {
	RequestId uint64
	Type      string
	Version   int
	Id        string
	Request   string
	Params    json.RawMessage
	Error     string
	ErrorCode string
	Response  json.RawMessage
}

type inMsgV1 struct {
	RequestId uint64                 `json:"request-id"`
	Type      string                 `json:"type"`
	Version   int                    `json:"version"`
	Id        string                 `json:"id"`
	Request   string                 `json:"request"`
	Params    json.RawMessage        `json:"params"`
	Error     string                 `json:"error"`
	ErrorCode string                 `json:"error-code"`
	ErrorInfo map[string]interface{} `json:"error-info"`
	Response  json.RawMessage        `json:"response"`
}

// outMsg holds an outgoing message.
type outMsgV0 struct {
	RequestId uint64
	Type      string      `json:",omitempty"`
	Version   int         `json:",omitempty"`
	Id        string      `json:",omitempty"`
	Request   string      `json:",omitempty"`
	Params    interface{} `json:",omitempty"`
	Error     string      `json:",omitempty"`
	ErrorCode string      `json:",omitempty"`
	Response  interface{} `json:",omitempty"`
}

type outMsgV1 struct {
	RequestId uint64                 `json:"request-id,omitempty"`
	Type      string                 `json:"type,omitempty"`
	Version   int                    `json:"version,omitempty"`
	Id        string                 `json:"id,omitempty"`
	Request   string                 `json:"request,omitempty"`
	Params    interface{}            `json:"params,omitempty"`
	Error     string                 `json:"error,omitempty"`
	ErrorCode string                 `json:"error-code,omitempty"`
	ErrorInfo map[string]interface{} `json:"error-info,omitempty"`
	Response  interface{}            `json:"response,omitempty"`
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
	var m json.RawMessage
	var version int
	err := c.conn.Receive(&m)
	if err == nil {
		logger.Tracef("<- %s", m)
		c.msg, version, err = c.readMessage(m)
	} else {
		logger.Tracef("<- error: %v (closing %v)", err, c.isClosing())
	}
	if err != nil {
		// If we've closed the connection, we may get a spurious error,
		// so ignore it.
		if c.isClosing() || err == io.EOF {
			return io.EOF
		}
		return errors.Annotate(err, "error receiving message")
	}
	hdr.RequestId = c.msg.RequestId
	hdr.Request = rpc.Request{
		Type:    c.msg.Type,
		Version: c.msg.Version,
		Id:      c.msg.Id,
		Action:  c.msg.Request,
	}
	hdr.Error = c.msg.Error
	hdr.ErrorCode = c.msg.ErrorCode
	hdr.ErrorInfo = c.msg.ErrorInfo
	hdr.Version = version
	return nil
}

func (c *Codec) readMessage(m json.RawMessage) (inMsgV1, int, error) {
	var msg inMsgV1
	if err := json.Unmarshal(m, &msg); err != nil {
		return msg, -1, errors.Trace(err)
	}
	// In order to support both new style tags (lowercase) and the old style tags (camelcase)
	// we look at the request id. The request id is always greater than one. If the value is
	// zero, it means that there wasn't a match for the "request-id" tag. This most likely
	// means that it was "RequestId" which was from the old style.
	if msg.RequestId == 0 {
		return c.readV0Message(m)
	}
	return msg, 1, nil
}

func (c *Codec) readV0Message(m json.RawMessage) (inMsgV1, int, error) {
	var msg inMsgV0
	if err := json.Unmarshal(m, &msg); err != nil {
		return inMsgV1{}, -1, errors.Trace(err)
	}
	return inMsgV1{
		RequestId: msg.RequestId,
		Type:      msg.Type,
		Version:   msg.Version,
		Id:        msg.Id,
		Request:   msg.Request,
		Params:    msg.Params,
		Error:     msg.Error,
		ErrorCode: msg.ErrorCode,
		Response:  msg.Response,
	}, 0, nil
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
	msg, err := response(hdr, body)
	if err != nil {
		return []byte(fmt.Sprintf("%q", err.Error()))
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return []byte(fmt.Sprintf("%q", "marshal error: "+err.Error()))
	}
	return data
}

func (c *Codec) WriteMessage(hdr *rpc.Header, body interface{}) error {
	msg, err := response(hdr, body)
	if err != nil {
		return errors.Trace(err)
	}
	if logger.IsTraceEnabled() {
		data, err := json.Marshal(msg)
		if err != nil {
			logger.Tracef("-> marshal error: %v", err)
			return err
		}
		logger.Tracef("-> %s", data)
	}
	return c.conn.Send(msg)
}

func response(hdr *rpc.Header, body interface{}) (interface{}, error) {
	switch hdr.Version {
	case 0:
		return newOutMsgV0(hdr, body), nil
	case 1:
		return newOutMsgV1(hdr, body), nil
	default:
		return nil, errors.Errorf("unsupported version %d", hdr.Version)
	}
}

// newOutMsgV0 fills out a outMsgV0 with information from the given
// header and body.
func newOutMsgV0(hdr *rpc.Header, body interface{}) outMsgV0 {
	result := outMsgV0{
		RequestId: hdr.RequestId,
		Type:      hdr.Request.Type,
		Version:   hdr.Request.Version,
		Id:        hdr.Request.Id,
		Request:   hdr.Request.Action,
		Error:     hdr.Error,
		ErrorCode: hdr.ErrorCode,
	}
	if hdr.IsRequest() {
		result.Params = body
	} else {
		result.Response = body
	}
	return result
}

// newOutMsgV1 fills out a outMsgV1 with information from the given header and
// body. This might look a lot like the v0 method, and that is because it is.
// However, since Go determines structs to be sufficiently different if the
// tags are different, we can't use the same code. Theoretically we could use
// reflect, but no.
func newOutMsgV1(hdr *rpc.Header, body interface{}) outMsgV1 {
	result := outMsgV1{
		RequestId: hdr.RequestId,
		Type:      hdr.Request.Type,
		Version:   hdr.Request.Version,
		Id:        hdr.Request.Id,
		Request:   hdr.Request.Action,
		Error:     hdr.Error,
		ErrorCode: hdr.ErrorCode,
		ErrorInfo: hdr.ErrorInfo,
	}
	if hdr.IsRequest() {
		result.Params = body
	} else {
		result.Response = body
	}
	return result
}
