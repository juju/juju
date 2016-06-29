// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424

import (
	stdtls "crypto/tls"
	"io"
	"net"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/standards/tls"
)

const (
	defaultSyslogTLSPort = "6514"
)

// Conn is the subset of net.Conn needed for a syslog client.
type Conn interface {
	io.Closer

	// Write writes the message to the connection.
	Write([]byte) (int, error)

	// SetWriteDeadline sets the absolute time after which any write
	// to the connection will time out.
	SetWriteDeadline(time.Time) error
}

// DialFunc is a function that may be used to open a network connection.
type DialFunc func(network, address string) (Conn, error)

func dialTimeoutFunc(timeout time.Duration) DialFunc {
	return func(network, address string) (Conn, error) {
		return net.DialTimeout(network, address, timeout)
	}
}

// TLSDialFunc returns a dial function that opens a TLS connection. If
// the address passed to the returned func does not include a port then
// the default syslog TLS port (6514) will be used.
func TLSDialFunc(cfg tls.Config, timeout time.Duration) (DialFunc, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	dial := func(network, address string) (Conn, error) {
		if network != "tcp" {
			return nil, errors.Errorf("unsupported network %q", network)
		}
		if _, _, err := net.SplitHostPort(address); err != nil {
			address = net.JoinHostPort(address, defaultSyslogTLSPort)
		}
		conn, err := tls.DialTCP(tls.DialOpts{
			Address:        address,
			TLSConfig:      cfg,
			ConnectTimeout: timeout,
		})
		if err != nil {
			return nil, errors.Annotate(err, "dialing TLS")
		}
		return conn, nil
	}
	return dial, nil
}

// ClientConfig is the configuration for a syslog client.
type ClientConfig struct {
	// MaxSize is the maximum allowed size for syslog messages sent
	// by the client. If not set then there is no maximum.
	MaxSize int

	// SendTImeout is the timeout that is used for each sent message.
	SendTimeout time.Duration
}

// Client is a wrapper around a network connection to which syslog
// messages will be sent.
type Client struct {
	maxSize int
	timeout time.Duration
	conn    Conn
}

// Open opens a syslog client to the given host address. If no dial
// func is provided then net.Dial is used.
func Open(host string, cfg ClientConfig, dial DialFunc) (*Client, error) {
	if dial == nil {
		dial = func(n, a string) (Conn, error) { return net.Dial(n, a) }
	}
	conn, err := dial("tcp", host)
	if err != nil {
		return nil, errors.Trace(err)
	}

	client := &Client{
		maxSize: cfg.MaxSize,
		timeout: cfg.SendTimeout,
		conn:    conn,
	}
	return client, nil
}

// Close closes the client's underlying connection.
func (client Client) Close() error {
	err := client.conn.Close()
	return errors.Trace(err)
}

// Send sends the syslog message over the client's connection.
func (client Client) Send(msg Message) error {
	data := client.serialize(msg)
	if err := client.send(data); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (client Client) serialize(msg Message) []byte {
	msgStr := msg.String()
	if client.maxSize > 0 && len(msgStr) > client.maxSize {
		msgStr = msgStr[:client.maxSize]
	}

	switch client.conn.(type) {
	case *net.TCPConn, *stdtls.Conn:
		msgStr += "\n"
	case *net.UDPConn:
		// For now do nothing.
	}
	return []byte(msgStr)
}

func (client Client) send(msg []byte) error {
	if client.timeout > 0 {
		deadline := time.Now().Add(client.timeout)
		if err := client.conn.SetWriteDeadline(deadline); err != nil {
			return errors.Trace(err)
		}
	}

	if _, err := client.conn.Write(msg); err != nil {
		return errors.Trace(err)
	}
	return nil
}
