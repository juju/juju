// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport

import (
	"net"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/v2/pubsub/apiserver"
)

const (
	// AddrTimeout is how long we'll wait for a good address to be
	// sent before timing out in the Addr call - this is better than
	// hanging indefinitely.
	AddrTimeout = 1 * time.Minute
)

var (
	// ErrAddressTimeout is used as the death reason when this transport dies because no good API address has been sent.
	ErrAddressTimeout = errors.New("timed out waiting for API address")
)

func newStreamLayer(
	localID raft.ServerID,
	hub *pubsub.StructuredHub,
	connections <-chan net.Conn,
	clk clock.Clock,
	dialer *Dialer,
) (*streamLayer, error) {
	l := &streamLayer{
		localID:     localID,
		hub:         hub,
		connections: connections,
		dialer:      dialer,

		addr:        make(chan net.Addr),
		addrChanges: make(chan string),
		clock:       clk,
	}
	// Watch for apiserver details changes, sending them
	// down the "addrChanges" channel. The worker loop
	// picks those up and makes the address available to
	// the "Addr()" method.
	unsubscribe, err := hub.Subscribe(apiserver.DetailsTopic, l.apiserverDetailsChanged)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Ask for the current details to be sent.
	req := apiserver.DetailsRequest{
		Requester: "raft-transport-stream-layer",
		LocalOnly: true,
	}
	if _, err := hub.Publish(apiserver.DetailsRequestTopic, req); err != nil {
		unsubscribe()
		return nil, errors.Trace(err)
	}

	l.tomb.Go(func() error {
		defer unsubscribe()
		return l.loop()
	})
	return l, nil
}

// streamLayer represents the connection between raft nodes.
//
// Partially based on code from https://github.com/CanonicalLtd/raft-http.
type streamLayer struct {
	tomb        tomb.Tomb
	localID     raft.ServerID
	hub         *pubsub.StructuredHub
	connections <-chan net.Conn
	dialer      *Dialer
	addr        chan net.Addr
	addrChanges chan string
	clock       clock.Clock
}

// Kill implements worker.Worker.
func (l *streamLayer) Kill() {
	l.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (l *streamLayer) Wait() error {
	return l.tomb.Wait()
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

var invalidAddr = tcpAddr("address.invalid:0")

// Addr returns the local address for the layer.
func (l *streamLayer) Addr() net.Addr {
	select {
	case <-l.tomb.Dying():
		return invalidAddr
	case <-l.clock.After(AddrTimeout):
		logger.Errorf("streamLayer.Addr timed out waiting for API address")
		// Stop this (and parent) worker.
		l.tomb.Kill(ErrAddressTimeout)
		return invalidAddr
	case addr := <-l.addr:
		return addr
	}
}

// Dial creates a new network connection.
func (l *streamLayer) Dial(addr raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	return l.dialer.Dial(addr, timeout)
}

func (l *streamLayer) loop() error {
	// Wait for the internal address of this agent,
	// and then send it out on l.addr whenever possible.
	var addr tcpAddr
	var out chan<- net.Addr
	for {
		select {
		case <-l.tomb.Dying():
			return tomb.ErrDying
		case newAddr := <-l.addrChanges:
			if newAddr == "" || newAddr == string(addr) {
				continue
			}
			addr = tcpAddr(newAddr)
			out = l.addr
		case out <- addr:
		}
	}
}

func (l *streamLayer) apiserverDetailsChanged(topic string, details apiserver.Details, err error) {
	if err != nil {
		l.tomb.Kill(err)
		return
	}
	var addr string
	for _, server := range details.Servers {
		if raft.ServerID(server.ID) != l.localID {
			continue
		}
		addr = server.InternalAddress
		break
	}
	select {
	case l.addrChanges <- addr:
	case <-l.tomb.Dying():
	}
}

// tcpAddr is an implementation of net.Addr which simply
// returns the address reported via pubsub. This avoids
// having to resolve the address just to get back the
// string representation of the address, which is all that
// the address is used for.
type tcpAddr string

// Network is part of the net.Addr interface.
func (a tcpAddr) Network() string {
	return "tcp"
}

// String is part of the net.Addr interface.
func (a tcpAddr) String() string {
	return string(a)
}
