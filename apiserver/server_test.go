// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/x509"
	"fmt"
	"net/http"
	stdtesting "testing"

	"github.com/gorilla/websocket"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	jujuhttp "github.com/juju/juju/internal/http"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

var fastDialOpts = api.DialOpts{}

func dialWebsocketFromURL(c *tc.C, server string, header http.Header) (*websocket.Conn, *http.Response, error) {
	// TODO(rogpeppe) merge this with the very similar dialWebsocket function.
	if header == nil {
		header = http.Header{}
	}
	header.Set("Origin", "http://localhost/")
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(coretesting.CACert)), tc.IsTrue)
	tlsConfig := jujuhttp.SecureTLSConfig()
	tlsConfig.RootCAs = caCerts
	tlsConfig.ServerName = "juju-apiserver"

	dialer := &websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	return dialer.Dial(server, header)
}

type serverSuite struct {
	jujutesting.ApiServerSuite
}

func TestServerSuite(t *stdtesting.T) {
	tc.Run(t, &serverSuite{})
}

func TestStub(t *stdtesting.T) {
	t.Skipf(`
This suite is missing tests for the following scenarios:
	- Valid machine login.
	- Test API server can listen on both IPv4 and IPv6.
	- Test stop
	- Test API login as a super user.
	- Test API server handler has correct permission login.
	- Test API server handler has correct permission login token (JWT flow).
	- Test API server teardown of initial model.
	- Test API server teardown of other model.
`)
}

func dialWebsocket(c *tc.C, addr, path string) (*websocket.Conn, error) {
	// TODO(rogpeppe) merge this with the very similar dialWebsocketFromURL function.
	url := fmt.Sprintf("wss://%s%s", addr, path)
	header := make(http.Header)
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(coretesting.CACert)), tc.IsTrue)
	tlsConfig := jujuhttp.SecureTLSConfig()
	tlsConfig.RootCAs = caCerts
	tlsConfig.ServerName = "anything"

	dialer := &websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	conn, _, err := dialer.Dial(url, header)
	return conn, err
}

func (s *serverSuite) TestNonCompatiblePathsAre404(c *tc.C) {
	// We expose the API at '/api', '/' (controller-only), and at '/ModelUUID/api'
	// for the correct location, but other paths should fail.
	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)

	// We have to use 'localhost' because that is what the TLS cert says.
	info := s.ControllerModelApiInfo()
	addr := fmt.Sprintf("localhost:%d", info.Ports()[0])

	// '/api' should be fine
	conn, err := dialWebsocket(c, addr, "/api")
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// '/`' should be fine
	conn, err = dialWebsocket(c, addr, "/")
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// '/model/MODELUUID/api' should be fine
	conn, err = dialWebsocket(c, addr, "/model/deadbeef-1234-5678-0123-0123456789ab/api")
	c.Assert(err, tc.ErrorIsNil)
	conn.Close()

	// '/randompath' is not ok
	conn, err = dialWebsocket(c, addr, "/randompath")
	// Unfortunately gorilla/websocket just returns bad handshake, it doesn't
	// give us any information (whether this was a 404 Not Found, Internal
	// Server Error, 200 OK, etc.)
	c.Assert(err, tc.ErrorMatches, `websocket: bad handshake`)
	c.Assert(conn, tc.IsNil)
}
