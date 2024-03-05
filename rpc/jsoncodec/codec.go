// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jsoncodec

import (
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"

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
	// Close closes the connection.
	Close() error
}

// Codec implements rpc.Codec for a connection.
type Codec struct {
	// msg holds the message that's just been read by ReadHeader, so
	// that the body can be read by ReadBody.
	msg     inMsgV1
	conn    JSONConn
	closing atomic.Bool
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

type inMsgV1 struct {
	RequestId  uint64                 `json:"request-id"`
	Type       string                 `json:"type"`
	Version    int                    `json:"version"`
	Id         string                 `json:"id"`
	Request    string                 `json:"request"`
	Params     json.RawMessage        `json:"params"`
	Error      string                 `json:"error"`
	ErrorCode  string                 `json:"error-code"`
	ErrorInfo  map[string]interface{} `json:"error-info"`
	Response   json.RawMessage        `json:"response"`
	TraceID    string                 `json:"trace-id"`
	SpanID     string                 `json:"span-id"`
	TraceFlags int                    `json:"trace-flags"`
}

type outMsgV1 struct {
	RequestId  uint64                 `json:"request-id,omitempty"`
	Type       string                 `json:"type,omitempty"`
	Version    int                    `json:"version,omitempty"`
	Id         string                 `json:"id,omitempty"`
	Request    string                 `json:"request,omitempty"`
	Params     interface{}            `json:"params,omitempty"`
	Error      string                 `json:"error,omitempty"`
	ErrorCode  string                 `json:"error-code,omitempty"`
	ErrorInfo  map[string]interface{} `json:"error-info,omitempty"`
	Response   interface{}            `json:"response,omitempty"`
	TraceID    string                 `json:"trace-id,omitempty"`
	SpanID     string                 `json:"span-id,omitempty"`
	TraceFlags int                    `json:"trace-flags,omitempty"`
}

// Close closes the underlying connection and sets the codec to
// closing mode, so that any further errors are ignored.
func (c *Codec) Close() error {
	c.closing.Swap(true)
	return c.conn.Close()
}

func (c *Codec) isClosing() bool {
	return c.closing.Load()
}

// ReadHeader reads the header from the connection.
func (c *Codec) ReadHeader(hdr *rpc.Header) error {
	var m json.RawMessage
	if err := c.conn.Receive(&m); err != nil {
		if logger.IsTraceEnabled() {
			logger.Tracef("<- error: %v (closing %v)", err, c.isClosing())
		}

		// If we've closed the connection, we may get a spurious error,
		// so ignore it.
		if c.isClosing() || err == io.EOF {
			return io.EOF
		}
		return errors.Annotate(err, "receiving message")
	}

	if logger.IsTraceEnabled() {
		logger.Tracef("<- %s", m)
	}
	var err error
	c.msg, err = readMessage(m)
	if err != nil {
		return errors.Annotate(err, "reading message")
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
	hdr.TraceID = c.msg.TraceID
	hdr.SpanID = c.msg.SpanID
	hdr.TraceFlags = c.msg.TraceFlags
	hdr.Version = 1
	return nil
}

// ReadBody reads the body from the connection.
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

// WriteMessage writes a message with the given header and body.
func (c *Codec) WriteMessage(hdr *rpc.Header, body interface{}) error {
	msg, err := response(hdr, body)
	if err != nil {
		return errors.Annotate(err, "writing message")
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

func readMessage(m json.RawMessage) (inMsgV1, error) {
	var msg inMsgV1
	if err := json.Unmarshal(m, &msg); err != nil {
		return msg, errors.Annotate(err, "unmarshalling message")
	}
	if msg.RequestId == 0 {
		return msg, errors.NotSupportedf("version 0")
	}
	return msg, nil
}

func response(hdr *rpc.Header, body interface{}) (interface{}, error) {
	switch hdr.Version {
	case 0:
		return nil, errors.NotSupportedf("version 0")
	case 1:
		return newOutMsgV1(hdr, body), nil
	default:
		return nil, errors.NotSupportedf("version %d", hdr.Version)
	}
}

// newOutMsgV1 fills out a outMsgV1 with information from the given header and
// body. This might look a lot like the v0 method, and that is because it is.
// However, since Go determines structs to be sufficiently different if the
// tags are different, we can't use the same code. Theoretically we could use
// reflect, but no.
func newOutMsgV1(hdr *rpc.Header, body interface{}) outMsgV1 {
	result := outMsgV1{
		RequestId:  hdr.RequestId,
		Type:       hdr.Request.Type,
		Version:    hdr.Request.Version,
		Id:         hdr.Request.Id,
		Request:    hdr.Request.Action,
		Error:      hdr.Error,
		ErrorCode:  hdr.ErrorCode,
		ErrorInfo:  hdr.ErrorInfo,
		TraceID:    hdr.TraceID,
		SpanID:     hdr.SpanID,
		TraceFlags: hdr.TraceFlags,
	}
	if hdr.IsRequest() {
		result.Params = body
	} else {
		result.Response = body
	}
	return result
}
