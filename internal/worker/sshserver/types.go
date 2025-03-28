// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"net"
	"time"

	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"
)

// chanConn fulfills the net.Conn interface without
// the tcpChan having to hold laddr or raddr directly.
type chanConn struct {
	gossh.Channel
	laddr, raddr net.Addr
}

// newChannelConn creates a struct that fulfills the net.Conn
// interface using the provided SSH channel. The client side
// of the connection should also be treated as a TCP pipe.
func newChannelConn(ch gossh.Channel) *chanConn {
	// Use a zero address for local and remote address.
	zeroAddr := &net.TCPAddr{
		IP:   net.IPv4zero,
		Port: 0,
	}
	return &chanConn{
		Channel: ch,
		laddr:   zeroAddr,
		raddr:   zeroAddr,
	}
}

// LocalAddr returns the local network address.
func (t *chanConn) LocalAddr() net.Addr {
	return t.laddr
}

// RemoteAddr returns the remote network address.
func (t *chanConn) RemoteAddr() net.Addr {
	return t.raddr
}

// SetDeadline sets the read and write deadlines associated
// with the connection.
func (t *chanConn) SetDeadline(deadline time.Time) error {
	if err := t.SetReadDeadline(deadline); err != nil {
		return err
	}
	return t.SetWriteDeadline(deadline)
}

// SetReadDeadline sets the read deadline.
// A zero value for t means Read will not time out.
// After the deadline, the error from Read will implement net.Error
// with Timeout() == true.
func (t *chanConn) SetReadDeadline(deadline time.Time) error {
	// for compatibility with previous version,
	// the error message contains "tcpChan"
	return errors.New("ssh: tcpChan: deadline not supported")
}

// SetWriteDeadline exists to satisfy the net.Conn interface
// but is not implemented by this type.  It always returns an error.
func (t *chanConn) SetWriteDeadline(deadline time.Time) error {
	return errors.New("ssh: tcpChan: deadline not supported")
}
