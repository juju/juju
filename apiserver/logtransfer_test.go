// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bufio"
	// "io/ioutil"
	"net/http"
	"net/url"
	// "os"
	// "path/filepath"
	// "runtime"
	// "time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	// "gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/apiserver/params"
	// coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

type logtransferSuite struct {
	authHTTPSuite
	userTag  names.Tag
	password string
	logs     loggo.TestWriter
}

var _ = gc.Suite(&logtransferSuite{})

func (s *logtransferSuite) SetUpTest(c *gc.C) {
	s.authHTTPSuite.SetUpTest(c)
	s.password = "jabberwocky"
	u := s.Factory.MakeUser(c, &factory.UserParams{Password: s.password})
	s.userTag = u.Tag()

	s.logs.Clear()
	writer := loggo.NewMinimumLevelWriter(&s.logs, loggo.INFO)
	c.Assert(loggo.RegisterWriter("logsink-tests", writer), jc.ErrorIsNil)
}

func (s *logtransferSuite) logtransferURL(c *gc.C, scheme string) *url.URL {
	controllerModel, err := s.State.ControllerModel()
	c.Assert(err, jc.ErrorIsNil)
	server := s.makeURL(c, scheme, "/model/"+controllerModel.UUID()+"/migration-logtransfer", nil)
	query := server.Query()
	query.Set("jujuclientversion", version.Current.String())
	server.RawQuery = query.Encode()
	return server
}

func (s *logtransferSuite) makeAuthHeader() http.Header {
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	header.Add(params.MigrationModelHeader, s.State.ModelUUID())
	return header
}

func (s *logtransferSuite) dialWebsocket(c *gc.C) *websocket.Conn {
	return s.dialWebsocketInternal(c, s.makeAuthHeader())
}

func (s *logtransferSuite) dialWebsocketInternal(c *gc.C, header http.Header) *websocket.Conn {
	server := s.logtransferURL(c, "wss").String()
	conn := dialWebsocketFromURL(c, server, header)
	s.AddCleanup(func(_ *gc.C) { conn.Close() })
	return conn
}

func (s *logtransferSuite) openWebsocketCustomPath(c *gc.C, path string) *bufio.Reader {
	server := s.logtransferURL(c, "wss")
	server.Path = path
	conn := dialWebsocketFromURL(c, server.String(), s.makeAuthHeader())
	return bufio.NewReader(conn)
}

func (s *logtransferSuite) TestRejectsBadModelUUID(c *gc.C) {
	reader := s.openWebsocketCustomPath(c, "/model/does-not-exist/migration-logtransfer")
	assertJSONError(c, reader, `unknown model: "does-not-exist"`)
	assertWebsocketClosed(c, reader)
}

func (s *logtransferSuite) TestRejectsNonControllerURL(c *gc.C) {
	reader := s.openWebsocketCustomPath(c, "/model/"+s.State.ModelUUID()+"/logsink")
	assertJSONError(c, reader, `not a controller: "`+s.State.ModelUUID()+`"`)
	assertWebsocketClosed(c, reader)
}

func (s *logtransferSuite) TestRejectsMissingModelHeader(c *gc.C) {
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	reader := bufio.NewReader(dialWebsocketFromURL(c, s.logtransferURL(c, "wss").String(), header))
	assertJSONError(c, reader, `missing migrating model header`)
	assertWebsocketClosed(c, reader)
}

func (s *logtransferSuite) TestRejectsBadMigratingModelUUID(c *gc.C) {
	header := utils.BasicAuthHeader(s.userTag.String(), s.password)
	header.Add(params.MigrationModelHeader, "does-not-exist")
	reader := bufio.NewReader(dialWebsocketFromURL(c, s.logtransferURL(c, "wss").String(), header))
	assertJSONError(c, reader, `unknown model: "does-not-exist"`)
	assertWebsocketClosed(c, reader)
}
