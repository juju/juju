// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport

import (
	"bufio"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"

	"github.com/juju/juju/v3/api"
)

// Dialer is a type that can be used for dialling a connection
// connecting to a raft endpoint using the configured path, and
// upgrading to a raft connection.
type Dialer struct {
	// APIInfo is used for authentication.
	APIInfo *api.Info

	// DialRaw returns a connection to the HTTP server
	// that is serving the raft endpoint.
	DialRaw func(raft.ServerAddress, time.Duration) (net.Conn, error)

	// Path is the path of the raft HTTP endpoint.
	Path string
}

// Dial dials a new raft network connection to the controller agent
// with the tag identified by the given address.
//
// Based on code from https://github.com/CanonicalLtd/raft-http.
func (d *Dialer) Dial(addr raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	request := &http.Request{
		Method:     "GET",
		URL:        &url.URL{Path: d.Path},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	request.Header.Set("Upgrade", "raft")
	if err := api.AuthHTTPRequest(request, d.APIInfo); err != nil {
		return nil, errors.Trace(err)
	}

	logger.Infof("dialing %s", addr)
	conn, err := d.DialRaw(addr, timeout)
	if err != nil {
		return nil, errors.Annotate(err, "dial failed")
	}

	if err := request.Write(conn); err != nil {
		return nil, errors.Annotate(err, "sending HTTP request failed")
	}

	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read response")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		if body, err := ioutil.ReadAll(response.Body); err == nil && len(body) != 0 {
			logger.Tracef("response: %s", body)
		}
		return nil, errors.Errorf(
			"expected status code %d, got %d",
			http.StatusSwitchingProtocols,
			response.StatusCode,
		)
	}
	if response.Header.Get("Upgrade") != "raft" {
		return nil, errors.New("missing or unexpected Upgrade header in response")
	}
	return conn, err
}
