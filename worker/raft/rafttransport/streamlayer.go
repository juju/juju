// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport

import (
	"net"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/names"
	"github.com/pkg/errors"
	"gopkg.in/tomb.v1"
)

func newStreamLayer(
	tag names.Tag,
	connections <-chan net.Conn,
	dialer *Dialer,
) *streamLayer {
	l := &streamLayer{
		addr:        jujuAddr{tag},
		connections: connections,
		dialer:      dialer,
	}
	go func() {
		defer l.tomb.Done()
		<-l.tomb.Dying()
		l.tomb.Kill(tomb.ErrDying)
	}()
	return l
}

// streamLayer represents the connection between raft nodes.
type streamLayer struct {
	tomb        tomb.Tomb
	addr        jujuAddr
	connections <-chan net.Conn
	dialer      *Dialer
}

// Accept waits for the next connection.
func (l *streamLayer) Accept() (net.Conn, error) {
	select {
	case <-l.tomb.Dying():
		return nil, errors.New("transport closed")
	case conn := <-l.connections:
		return conn, nil
	}
}

// Close closes the layer.
func (l *streamLayer) Close() error {
	l.tomb.Kill(nil)
	return l.tomb.Wait()
}

// Addr returns the local address for the layer.
func (l *streamLayer) Addr() net.Addr {
	return l.addr
}

// Dial creates a new network connection.
func (l *streamLayer) Dial(addr raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	return l.dialer.Dial(addr, timeout)
}

// jujuAddr implements net.Addr in terms of a tag.
type jujuAddr struct {
	names.Tag
}

// Network is part of the net.Addr interface.
func (jujuAddr) Network() string {
	return "juju"
}
