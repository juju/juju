// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bufio"
	"io/ioutil"
	"net/http"
	"net/url"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing/factory"
)

type debugLogDBSuite struct {
	authHTTPSuite
}

var _ = gc.Suite(&debugLogDBSuite{})

// See debuglog_db_internal_test.go for DB specific unit tests and the
// featuretests package for an end-to-end integration test.

func (s *debugLogDBSuite) TestBadParams(c *gc.C) {
	reader := s.openWebsocket(c, url.Values{"maxLines": {"foo"}})
	assertJSONError(c, reader, `maxLines value "foo" is not a valid unsigned number`)
	assertWebsocketClosed(c, reader)
}

func (s *debugLogDBSuite) TestWithHTTP(c *gc.C) {
	uri := s.logURL(c, "http", nil).String()
	s.sendRequest(c, httpRequestParams{
		method:      "GET",
		url:         uri,
		expectError: `.*malformed HTTP response.*`,
	})
}

func (s *debugLogDBSuite) TestWithHTTPS(c *gc.C) {
	uri := s.logURL(c, "https", nil).String()
	response := s.sendRequest(c, httpRequestParams{method: "GET", url: uri})
	c.Assert(response.StatusCode, gc.Equals, http.StatusBadRequest)
}

func (s *debugLogDBSuite) TestNoAuth(c *gc.C) {
	conn := s.dialWebsocketInternal(c, nil, nil)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	assertJSONError(c, reader, "no credentials provided")
	assertWebsocketClosed(c, reader)
}

func (s *debugLogDBSuite) TestUnitLoginsRejected(c *gc.C) {
	u, password := s.Factory.MakeUnitReturningPassword(c, nil)
	header := utils.BasicAuthHeader(u.Tag().String(), password)
	conn := s.dialWebsocketInternal(c, nil, header)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	assertJSONError(c, reader, "tag kind unit not valid")
	assertWebsocketClosed(c, reader)
}

var noResultsPlease = url.Values{"maxLines": {"0"}, "noTail": {"true"}}

func (s *debugLogDBSuite) TestUserLoginsAccepted(c *gc.C) {
	u := s.Factory.MakeUser(c, &factory.UserParams{
		Name:     "oryx",
		Password: "gardener",
	})
	header := utils.BasicAuthHeader(u.Tag().String(), "gardener")
	conn := s.dialWebsocketInternal(c, noResultsPlease, header)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	result := readJSONErrorLine(c, reader)
	c.Assert(result.Error, gc.IsNil)
	_, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *debugLogDBSuite) TestMachineLoginsAccepted(c *gc.C) {
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "foo-nonce",
	})
	header := utils.BasicAuthHeader(m.Tag().String(), password)
	header.Add(params.MachineNonceHeader, "foo-nonce")
	conn := s.dialWebsocketInternal(c, noResultsPlease, header)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	result := readJSONErrorLine(c, reader)
	c.Assert(result.Error, gc.IsNil)
	_, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *debugLogDBSuite) openWebsocket(c *gc.C, values url.Values) *bufio.Reader {
	conn := s.dialWebsocket(c, values)
	s.AddCleanup(func(_ *gc.C) { conn.Close() })
	return bufio.NewReader(conn)
}

func (s *debugLogDBSuite) openWebsocketCustomPath(c *gc.C, path string) *bufio.Reader {
	server := s.logURL(c, "wss", nil)
	server.Path = path
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	conn := dialWebsocketFromURL(c, server.String(), header)
	s.AddCleanup(func(_ *gc.C) { conn.Close() })
	return bufio.NewReader(conn)
}

func (s *debugLogDBSuite) logURL(c *gc.C, scheme string, queryParams url.Values) *url.URL {
	return s.makeURL(c, scheme, "/log", queryParams)
}

func (s *debugLogDBSuite) dialWebsocket(c *gc.C, queryParams url.Values) *websocket.Conn {
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	return s.dialWebsocketInternal(c, queryParams, header)
}

func (s *debugLogDBSuite) dialWebsocketInternal(c *gc.C, queryParams url.Values, header http.Header) *websocket.Conn {
	server := s.logURL(c, "wss", queryParams).String()
	return dialWebsocketFromURL(c, server, header)
}
