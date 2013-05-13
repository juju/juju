package jsoncodec

import (
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"io"
	"launchpad.net/juju-core/rpc"
)

// NewWS returns an rpc codec that uses conn to send and receive
// messages.
func NewWS(conn *websocket.Conn) rpc.Codec {
	return New(NewWSJSONConn(conn))
}

// NewRWC returns an rpc codec that uses rwc to send and receive
// messages.
func NewRWC(rwc io.ReadWriteCloser) rpc.Codec {
	return New(NewRWCJSONConn(rwc))
}

type wsJSONConn struct {
	conn *websocket.Conn
}

func (conn wsJSONConn) Send(msg interface{}) error {
	return websocket.JSON.Send(conn.conn, msg)
}

func (conn wsJSONConn) Receive(msg interface{}) error {
	return websocket.JSON.Receive(conn.conn, msg)
}

func (conn wsJSONConn) Close() error {
	return conn.conn.Close()
}

// NewWSJSONConn returns a JSONConn that reads and
// writes JSON messages to the given websocket connection.
func NewWSJSONConn(conn *websocket.Conn) JSONConn {
	return wsJSONConn{conn}
}

type rwcConn struct {
	enc  *json.Encoder
	dec  *json.Decoder
	conn io.ReadWriteCloser
}

func (conn *rwcConn) Send(msg interface{}) error {
	return conn.enc.Encode(msg)
}

func (conn *rwcConn) Receive(msg interface{}) error {
	return conn.dec.Decode(msg)
}

func (conn *rwcConn) Close() error {
	return conn.conn.Close()
}

// NewRWCJSONConn returns a JSONConn that reads and
// writes JSON messages to the given ReadWriteCloser.
func NewRWCJSONConn(rwc io.ReadWriteCloser) JSONConn {
	return &rwcConn{
		enc:  json.NewEncoder(rwc),
		dec:  json.NewDecoder(rwc),
		conn: rwc,
	}
}
