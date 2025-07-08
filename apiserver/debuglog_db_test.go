// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	jujuhttp "github.com/juju/http/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/websocket/websockettest"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing/factory"
)

type debugLogDBSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&debugLogDBSuite{})

// See debuglog_db_internal_test.go for DB specific unit tests and the
// featuretests package for an end-to-end integration test.

func (s *debugLogDBSuite) TestBadParams(c *gc.C) {
	conn := s.dialWebsocket(c, url.Values{"maxLines": {"foo"}})
	defer conn.Close()

	websockettest.AssertJSONError(c, conn, `maxLines value "foo" is not a valid unsigned number`)
	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *debugLogDBSuite) TestWithHTTP(c *gc.C) {
	uri := s.logURL("http", nil).String()
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          uri,
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *debugLogDBSuite) TestNoAuth(c *gc.C) {
	conn, _, err := s.dialWebsocketInternal(c, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	websockettest.AssertJSONError(c, conn, "authentication failed: no credentials provided")
	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *debugLogDBSuite) TestUnitLoginsRejected(c *gc.C) {
	u, password := s.Factory.MakeUnitReturningPassword(c, nil)
	header := jujuhttp.BasicAuthHeader(u.Tag().String(), password)

	conn, _, err := s.dialWebsocketInternal(c, nil, header)
	c.Assert(err, jc.ErrorIsNil)

	websockettest.AssertJSONError(c, conn, "authorization failed: permission denied")
	websockettest.AssertWebsocketClosed(c, conn)
}

var noResultsPlease = url.Values{"maxLines": {"0"}, "noTail": {"true"}}

func (s *debugLogDBSuite) TestUserLoginAccepted(c *gc.C) {
	u := s.Factory.MakeUser(c, &factory.UserParams{
		Name:     "oryx",
		Password: "gardener",
		Access:   permission.ReadAccess,
	})
	header := jujuhttp.BasicAuthHeader(u.Tag().String(), "gardener")
	conn, _, err := s.dialWebsocketInternal(c, noResultsPlease, header)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn, gc.NotNil)
	defer conn.Close()

	result := websockettest.ReadJSONErrorLine(c, conn)
	c.Assert(result.Error, gc.IsNil)
}

func (s *debugLogDBSuite) TestUserLoginRejected(c *gc.C) {
	u := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "oryx",
		Password:    "gardener",
		NoModelUser: true,
	})
	header := jujuhttp.BasicAuthHeader(u.Tag().String(), "gardener")
	conn, _, err := s.dialWebsocketInternal(c, noResultsPlease, header)
	c.Assert(err, jc.ErrorIsNil)

	websockettest.AssertJSONError(c, conn, "authorization failed: permission denied")
	websockettest.AssertWebsocketClosed(c, conn)
}

func (s *debugLogDBSuite) TestMachineLoginsAccepted(c *gc.C) {
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "foo-nonce",
	})
	header := jujuhttp.BasicAuthHeader(m.Tag().String(), password)
	header.Add(params.MachineNonceHeader, "foo-nonce")
	conn, _, err := s.dialWebsocketInternal(c, noResultsPlease, header)
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	result := websockettest.ReadJSONErrorLine(c, conn)
	c.Assert(result.Error, gc.IsNil)
}

func (s *debugLogDBSuite) logURL(scheme string, queryParams url.Values) *url.URL {
	url := s.URL("/log", queryParams)
	url.Scheme = scheme
	return url
}

func (s *debugLogDBSuite) dialWebsocket(c *gc.C, queryParams url.Values) *websocket.Conn {
	header := jujuhttp.BasicAuthHeader(s.Owner.String(), ownerPassword)
	conn, _, err := s.dialWebsocketInternal(c, queryParams, header)
	c.Assert(err, jc.ErrorIsNil)
	return conn
}

func (s *debugLogDBSuite) dialWebsocketInternal(
	c *gc.C, queryParams url.Values, header http.Header,
) (*websocket.Conn, *http.Response, error) {
	server := s.logURL("wss", queryParams).String()
	return dialWebsocketFromURL(c, server, header)
}
