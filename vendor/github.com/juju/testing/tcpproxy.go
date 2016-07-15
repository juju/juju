// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	"io"
	"net"
	"sync"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

// TCPProxy is a simple TCP proxy that can be used
// to deliberately break TCP connections.
type TCPProxy struct {
	listener net.Listener
	// mu guards the fields below it.
	mu sync.Mutex
	// closed holds whether the proxy has been closed.
	closed bool
	// conns holds all connections that have been made.
	conns []io.Closer
}

// NewTCPProxy runs a proxy that copies to and from
// the given remote TCP address. When the proxy
// is closed, its listener and all connections will be closed.
func NewTCPProxy(c *gc.C, remoteAddr string) *TCPProxy {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, jc.ErrorIsNil)
	p := &TCPProxy{
		listener: listener,
	}
	go func() {
		for {
			client, err := p.listener.Accept()
			if err != nil {
				if !p.isClosed() {
					c.Error("cannot accept: %v", err)
				}
				return
			}
			p.addConn(client)
			server, err := net.Dial("tcp", remoteAddr)
			if err != nil {
				if !p.isClosed() {
					c.Error("cannot dial remote address: %v", err)
				}
				return
			}
			p.addConn(server)
			go stream(client, server)
			go stream(server, client)
		}
	}()
	return p
}

func (p *TCPProxy) addConn(c net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		c.Close()
	} else {
		p.conns = append(p.conns, c)
	}
}

// Close closes the TCPProxy and any connections that
// are currently active.
func (p *TCPProxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.listener.Close()
	for _, c := range p.conns {
		c.Close()
	}
	return nil
}

// CloseConns closes all the connections that are
// currently active. The proxy itself remains active.
func (p *TCPProxy) CloseConns() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		c.Close()
	}
}

// Addr returns the TCP address of the proxy. Dialing
// this address will cause a connection to be made
// to the remote address; any data written will be
// written there, and any data read from the remote
// address will be available to read locally.
func (p *TCPProxy) Addr() string {
	// Note: this only works because we explicitly listen on 127.0.0.1 rather
	// than the wildcard address.
	return p.listener.Addr().String()
}

func (p *TCPProxy) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func stream(dst io.WriteCloser, src io.ReadCloser) {
	defer dst.Close()
	defer src.Close()
	io.Copy(dst, src)
}
