// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jsoncodec

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
)

// NewWebsocket returns an rpc codec that uses the given websocket
// connection to send and receive messages.
func NewWebsocket(conn *websocket.Conn) *Codec {
	return New(NewWebsocketConn(conn))
}

type wsJSONConn struct {
	conn *websocket.Conn
	// gorilla websockets can have at most one concurrent writer, and
	// one concurrent reader.
	writeMutex sync.Mutex
	readMutex  sync.Mutex
}

// NewWebsocketConn returns a JSONConn implementation
// that uses the given connection for transport.
func NewWebsocketConn(conn *websocket.Conn) JSONConn {
	return &wsJSONConn{conn: conn}
}

func (conn *wsJSONConn) Send(msg interface{}) error {
	conn.writeMutex.Lock()
	defer conn.writeMutex.Unlock()
	return conn.conn.WriteJSON(msg)
}

func (conn *wsJSONConn) Receive(msg interface{}) error {
	conn.readMutex.Lock()
	defer conn.readMutex.Unlock()
	// When receiving a message, if error has been closed from the other
	// side, wrap with io.EOF as this is the expected error.
	err := conn.conn.ReadJSON(msg)
	if err != nil {
		if websocket.IsCloseError(err,
			websocket.CloseNormalClosure,
			websocket.CloseGoingAway,
			websocket.CloseNoStatusReceived,
			websocket.CloseAbnormalClosure) {
			err = errors.Wrap(err, io.EOF)
		}
	}
	return err
}

func (conn *wsJSONConn) Close() error {
	// Tell the other end we are closing.
	conn.writeMutex.Lock()
	_ = conn.conn.WriteMessage(websocket.CloseMessage, []byte{})
	conn.writeMutex.Unlock()
	return conn.conn.Close()
}

// NewNet returns an rpc codec that uses the given connection
// to send and receive messages.
func NewNet(conn io.ReadWriteCloser) *Codec {
	return New(NetJSONConn(conn))
}

func NetJSONConn(conn io.ReadWriteCloser) JSONConn {
	return &netConn{
		enc:  json.NewEncoder(conn),
		dec:  json.NewDecoder(conn),
		conn: conn,
	}
}

type netConn struct {
	enc  *json.Encoder
	dec  *json.Decoder
	conn io.ReadWriteCloser
}

func (conn *netConn) Send(msg interface{}) error {
	return conn.enc.Encode(msg)
}

func (conn *netConn) Receive(msg interface{}) error {
	return conn.dec.Decode(msg)
}

func (conn *netConn) Close() error {
	return conn.conn.Close()
}
