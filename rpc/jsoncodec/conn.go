// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jsoncodec

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/juju/juju/internal/errors"
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
	// enc and dec are reused for every message, guarded by writeMutex
	// and readMutex respectively. They are reset to point at the
	// websocket frame of the message currently being processed.
	enc *jsontext.Encoder
	dec *jsontext.Decoder
}

// NewWebsocketConn returns a JSONConn implementation
// that uses the given connection for transport.
func NewWebsocketConn(conn *websocket.Conn) JSONConn {
	return &wsJSONConn{
		conn: conn,
		enc:  jsontext.NewEncoder(io.Discard, options),
		dec:  jsontext.NewDecoder(eofReader{}, options),
	}
}

// eofReader is an io.Reader that is always at EOF. It is used as the
// initial target of a reusable decoder.
type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }

func (conn *wsJSONConn) Send(msg any) error {
	conn.writeMutex.Lock()
	defer conn.writeMutex.Unlock()
	w, err := conn.conn.NextWriter(websocket.TextMessage)
	if err != nil {
		return err
	}
	// Each message is a single JSON value terminated by a newline,
	// matching the output of encoding/json.Encoder that was used
	// previously.
	conn.enc.Reset(w, options)
	marshalErr := json.MarshalEncode(conn.enc, msg)
	// Close the writer even if marshalling fails so that the
	// frame is flushed and the connection is left in a usable
	// state for the next message.
	closeErr := w.Close()
	if marshalErr != nil {
		return marshalErr
	}
	return closeErr
}

func (conn *wsJSONConn) Receive(msg any) error {
	conn.readMutex.Lock()
	defer conn.readMutex.Unlock()
	// When receiving a message, if error has been closed from the other
	// side, wrap with io.EOF as this is the expected error.
	_, r, err := conn.conn.NextReader()
	if err != nil {
		if websocket.IsCloseError(err,
			websocket.CloseNormalClosure,
			websocket.CloseGoingAway,
			websocket.CloseNoStatusReceived,
			websocket.CloseAbnormalClosure) {
			err = errors.Errorf(
				"reading json message: %w", err,
			).Add(io.EOF)
		}
		return err
	}
	// Each websocket message holds exactly one JSON value.
	conn.dec.Reset(r, options)
	err = json.UnmarshalDecode(conn.dec, msg)
	if errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	return err
}

const closingIODeadline = 10 * time.Second

func (conn *wsJSONConn) Close() error {
	c := conn.conn.NetConn()

	// After we close, all readers and writers
	// must be forced to unblock immediately.
	defer func() {
		_ = c.SetDeadline(time.Now())
	}()

	var closeErr error

	closedNormally := false
	if err := conn.writeClose(); err == nil {
		closedNormally = conn.readClose()
	} else if websocket.IsUnexpectedCloseError(err) {
		// The websocket connection was already closed by the other side.
		closedNormally = true
	} else {
		closeErr = err
	}

	// The underlying connection for the socket is a tls.Conn.
	// This sends a TLS close notification message to the peer.
	type closer interface {
		CloseWrite() error
	}
	if cl, ok := c.(closer); ok {
		err := cl.CloseWrite()
		if err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}

	// Now get the inner TCPConn from the TLS conn.
	// Use it to disable keep-alives, drop any unsent/unacked data,
	// and send FIN to the peer.
	type netConner interface {
		NetConn() net.Conn
	}
	if nc, ok := c.(netConner); ok {
		if tcpConn, ok := nc.NetConn().(*net.TCPConn); ok {
			_ = tcpConn.SetKeepAlive(false)
			_ = tcpConn.SetLinger(0)
			_ = tcpConn.CloseWrite()
		}
	}

	if err := conn.conn.Close(); errors.Is(err, syscall.EPIPE) {
		// This is expected due to tls.Conn writing on every Close and the local
		// socket having been shutdown to writing above.
		// See net.TCPConn.CloseWrite.
		// See tls.Conn.Close.
	} else if err != nil {
		closeErr = errors.Join(closeErr, err)
	}

	if !closedNormally {
		return closeErr
	}

	// If the websocket closed normally, a possible send/recv error during a
	// call to close is expected, since it is possible the other side has
	// already gone away, or the operation takes longer than the currently set
	// deadline, it is safe to discard this error as the other side has already
	// acknowledged the close.
	if errors.Is(closeErr, syscall.ECONNRESET) ||
		errors.Is(closeErr, syscall.ETIMEDOUT) ||
		errors.Is(closeErr, os.ErrDeadlineExceeded) {
		return nil
	}

	return closeErr
}

// WriteClose sets a write deadline to start a count-down for any existing
// writers. It then sends the socket close message.
func (conn *wsJSONConn) writeClose() error {
	_ = conn.conn.NetConn().SetWriteDeadline(time.Now().Add(closingIODeadline))

	conn.writeMutex.Lock()
	defer conn.writeMutex.Unlock()

	return conn.conn.WriteMessage(
		websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

// readClose sets a read deadline to start a count-down for any existing
// readers. It then attempts to drain all remaining reads looking for the
// socket close acknowledgement. If the closure was normal, true is returned.
func (conn *wsJSONConn) readClose() bool {
	_ = conn.conn.NetConn().SetReadDeadline(time.Now().Add(closingIODeadline))

	conn.readMutex.Lock()
	defer conn.readMutex.Unlock()

	closedNormally := false
	conn.conn.SetCloseHandler(func(code int, text string) error {
		closedNormally = true
		// Since this websocket was the closer, a close message does not need to
		// be reciprocated and should not be attempted.
		return nil
	})

	for {
		_, _, err := conn.conn.NextReader()
		if websocket.IsUnexpectedCloseError(err) {
			break
		} else if err != nil && closedNormally {
			break
		} else if err != nil {
			logger.Debugf(context.TODO(), "waiting for websocket close message: %v", err)
			break
		}
	}

	return closedNormally
}

// NewNet returns an rpc codec that uses the given connection
// to send and receive messages.
func NewNet(conn io.ReadWriteCloser) *Codec {
	return New(NetJSONConn(conn))
}

func NetJSONConn(conn io.ReadWriteCloser) JSONConn {
	return &netConn{
		enc:  jsontext.NewEncoder(conn, options),
		dec:  jsontext.NewDecoder(conn, options),
		conn: conn,
	}
}

type netConn struct {
	mu   sync.Mutex
	enc  *jsontext.Encoder
	dec  *jsontext.Decoder
	conn io.ReadWriteCloser
}

func (conn *netConn) Send(msg any) error {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return json.MarshalEncode(conn.enc, msg)
}

func (conn *netConn) Receive(msg any) error {
	return json.UnmarshalDecode(conn.dec, msg)
}

func (conn *netConn) Close() error {
	return conn.conn.Close()
}
