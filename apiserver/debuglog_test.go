// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bufio"
	"net/http"
	"net/url"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing/factory"
)

// debugLogBaseSuite has tests that should be run for both the file
// and DB based variants of debuglog, as well as some test helpers.
type debugLogBaseSuite struct {
	userAuthHttpSuite
}

func (s *debugLogBaseSuite) TestBadParams(c *gc.C) {
	reader := s.openWebsocket(c, url.Values{"maxLines": {"foo"}})
	assertJSONError(c, reader, `maxLines value "foo" is not a valid unsigned number`)
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogBaseSuite) TestWithHTTP(c *gc.C) {
	uri := s.logURL(c, "http", nil).String()
	_, err := s.sendRequest(c, "", "", "GET", uri, "", nil)
	c.Assert(err, gc.ErrorMatches, `.*malformed HTTP response.*`)
}

func (s *debugLogBaseSuite) TestWithHTTPS(c *gc.C) {
	uri := s.logURL(c, "https", nil).String()
	response, err := s.sendRequest(c, "", "", "GET", uri, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.StatusCode, gc.Equals, http.StatusBadRequest)
}

func (s *debugLogBaseSuite) TestNoAuth(c *gc.C) {
	conn := s.dialWebsocketInternal(c, nil, nil)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	assertJSONError(c, reader, "auth failed: invalid request format")
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogBaseSuite) TestAgentLoginsRejected(c *gc.C) {
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "foo-nonce",
	})
	header := utils.BasicAuthHeader(m.Tag().String(), password)
	header.Add("X-Juju-Nonce", "foo-nonce")
	conn := s.dialWebsocketInternal(c, nil, header)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	assertJSONError(c, reader, "auth failed: invalid entity name or password")
	s.assertWebsocketClosed(c, reader)
}

func (s *debugLogBaseSuite) openWebsocket(c *gc.C, values url.Values) *bufio.Reader {
	conn := s.dialWebsocket(c, values)
	s.AddCleanup(func(_ *gc.C) { conn.Close() })
	return bufio.NewReader(conn)
}

func (s *debugLogBaseSuite) openWebsocketCustomPath(c *gc.C, path string) *bufio.Reader {
	server := s.logURL(c, "wss", nil)
	server.Path = path
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	conn := s.dialWebsocketFromURL(c, server.String(), header)
	s.AddCleanup(func(_ *gc.C) { conn.Close() })
	return bufio.NewReader(conn)
}

func (s *debugLogBaseSuite) logURL(c *gc.C, scheme string, queryParams url.Values) *url.URL {
	return s.makeURL(c, scheme, "/log", queryParams)
}

func (s *debugLogBaseSuite) dialWebsocket(c *gc.C, queryParams url.Values) *websocket.Conn {
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	return s.dialWebsocketInternal(c, queryParams, header)
}

func (s *debugLogBaseSuite) dialWebsocketInternal(c *gc.C, queryParams url.Values, header http.Header) *websocket.Conn {
	server := s.logURL(c, "wss", queryParams).String()
	return s.dialWebsocketFromURL(c, server, header)
}
