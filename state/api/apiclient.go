// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"code.google.com/p/go.net/websocket"
	"crypto/tls"
	"crypto/x509"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/rpc/jsoncodec"
	"launchpad.net/juju-core/utils"
	"time"
)

type State struct {
	client *rpc.Conn
	conn   *websocket.Conn
}

// Info encapsulates information about a server holding juju state and
// can be used to make a connection to it.
type Info struct {
	// Addrs holds the addresses of the state servers.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert []byte

	// Tag holds the name of the entity that is connecting.
	// If this and the password are empty, no login attempt will be made
	// (this is to allow tests to access the API to check that operations
	// fail when not logged in).
	Tag string

	// Password holds the password for the administrator or connecting entity.
	Password string
}

var openAttempt = utils.AttemptStrategy{
	Total: 5 * time.Minute,
	Delay: 500 * time.Millisecond,
}

func Open(info *Info) (*State, error) {
	// TODO Select a random address from info.Addrs
	// and only fail when we've tried all the addresses.
	// TODO what does "origin" really mean, and is localhost always ok?
	cfg, err := websocket.NewConfig("wss://"+info.Addrs[0]+"/", "http://localhost/")
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	xcert, err := cert.ParseCert(info.CACert)
	if err != nil {
		return nil, err
	}
	pool.AddCert(xcert)
	cfg.TlsConfig = &tls.Config{
		RootCAs:    pool,
		ServerName: "anything",
	}
	var conn *websocket.Conn
	for a := openAttempt.Start(); a.Next(); {
		log.Infof("state/api: dialing %q", cfg.Location)
		conn, err = websocket.DialConfig(cfg)
		if err == nil {
			break
		}
		log.Errorf("state/api: %v", err)
	}
	if err != nil {
		return nil, err
	}
	log.Infof("state/api: connection established")

	client := rpc.NewConn(jsoncodec.NewWebsocket(conn))
	client.Start()
	st := &State{
		client: client,
		conn:   conn,
	}
	if info.Tag != "" || info.Password != "" {
		if err := st.Login(info.Tag, info.Password); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return st, nil
}

func (s *State) call(objType, id, request string, params, response interface{}) error {
	err := s.client.Call(objType, id, request, params, response)
	return clientError(err)
}

func (s *State) Close() error {
	return s.client.Close()
}

// SetDeadlines set the connection's network read and write deadlines.
func (s *State) SetDeadline(t time.Time) error {
	return s.conn.SetDeadline(t)
}

// RPCClient returns the RPC client for the state, so that testing
// functions can tickle parts of the API that the conventional entry
// points don't reach. This is exported for testing purposes only.
func (s *State) RPCClient() *rpc.Conn {
	return s.client
}
