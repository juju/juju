// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	"io"
	"net"
	"sync"

	"github.com/juju/tc"
)

// TCPProxy is a simple TCP proxy that can be used
// to deliberately break TCP connections.
type TCPProxy struct {
	listener net.Listener
	// mu guards the fields below it.
	mu sync.Mutex
	// stopStart holds a condition variable that broadcasts changes
	// in the paused state.
	stopStart sync.Cond
	// closed holds whether the proxy has been closed.
	closed bool
	// paused holds whether the proxy has been paused.
	paused bool
	// conns holds all connections that have been made.
	conns []io.Closer
}

// NewTCPProxy runs a proxy that copies to and from
// the given remote TCP address. When the proxy
// is closed, its listener and all connections will be closed.
func NewTCPProxy(c *tc.C, remoteAddr string) *TCPProxy {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, tc.ErrorIsNil)
	p := &TCPProxy{
		listener: listener,
	}
	p.stopStart.L = &p.mu
	go func() {
		for {
			client, err := p.listener.Accept()
			if err != nil {
				if !p.isClosed() {
					c.Errorf("cannot accept: %v", err)
				}
				return
			}
			p.addConn(client)
			server, err := net.Dial("tcp", remoteAddr)
			if err != nil {
				if !p.isClosed() {
					c.Errorf("cannot dial remote address: %v", err)
				}
				return
			}
			p.addConn(server)
			go p.stream(client, server)
			go p.stream(server, client)
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

// PauseConns stops all traffic flowing through the proxy.
func (p *TCPProxy) PauseConns() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = true
	p.stopStart.Broadcast()
}

// ResumeConns resumes sending traffic through the proxy.
func (p *TCPProxy) ResumeConns() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = false
	p.stopStart.Broadcast()
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

func (p *TCPProxy) stream(dst io.WriteCloser, src io.ReadCloser) {
	defer dst.Close()
	defer src.Close()
	buf := make([]byte, 32*1024)
	for {
		nr, err := src.Read(buf)
		p.mu.Lock()
		for p.paused {
			p.stopStart.Wait()
		}
		p.mu.Unlock()
		if nr > 0 {
			_, err := dst.Write(buf[0:nr])
			if err != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
}
