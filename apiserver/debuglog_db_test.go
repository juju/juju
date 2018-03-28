// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/websocket/websockettest"
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
		Method:      "GET",
		URL:         uri,
		ExpectError: `.*malformed HTTP response.*`,
	})
}

func (s *debugLogDBSuite) TestWithHTTPS(c *gc.C) {
	uri := s.logURL("https", nil).String()
	response := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	defer response.Body.Close()
	c.Assert(response.StatusCode, gc.Equals, http.StatusUnauthorized)
	out, err := ioutil.ReadAll(response.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *debugLogDBSuite) TestNoAuth(c *gc.C) {
	conn, resp, err := s.dialWebsocketInternal(c, nil, nil)
	c.Assert(err, gc.Equals, websocket.ErrBadHandshake)
	c.Assert(conn, gc.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusUnauthorized)
	out, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *debugLogDBSuite) TestUnitLoginsRejected(c *gc.C) {
	u, password := s.Factory.MakeUnitReturningPassword(c, nil)
	header := utils.BasicAuthHeader(u.Tag().String(), password)
	conn, resp, err := s.dialWebsocketInternal(c, nil, header)
	c.Assert(err, gc.Equals, websocket.ErrBadHandshake)
	c.Assert(conn, gc.IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
	out, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, "authorization failed: tag kind unit not valid\n")
}

var noResultsPlease = url.Values{"maxLines": {"0"}, "noTail": {"true"}}

func (s *debugLogDBSuite) TestUserLoginsAccepted(c *gc.C) {
	u := s.Factory.MakeUser(c, &factory.UserParams{
		Name:     "oryx",
		Password: "gardener",
	})
	header := utils.BasicAuthHeader(u.Tag().String(), "gardener")
	conn, _, err := s.dialWebsocketInternal(c, noResultsPlease, header)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn, gc.NotNil)
	defer conn.Close()

	result := websockettest.ReadJSONErrorLine(c, conn)
	c.Assert(result.Error, gc.IsNil)
}

func (s *debugLogDBSuite) TestMachineLoginsAccepted(c *gc.C) {
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "foo-nonce",
	})
	header := utils.BasicAuthHeader(m.Tag().String(), password)
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
	header := utils.BasicAuthHeader(s.Owner.String(), ownerPassword)
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
