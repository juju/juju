// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"

	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// authHttpSuite provides helpers for testing HTTP "streaming" style APIs.
type authHttpSuite struct {
	jujutesting.JujuConnSuite
	envUUID string
}

func (s *authHttpSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.envUUID = s.State.EnvironUUID()
}

func (s *authHttpSuite) baseURL(c *gc.C) *url.URL {
	info := s.APIInfo(c)
	return &url.URL{
		Scheme: "https",
		Host:   info.Addrs[0],
		Path:   "",
	}
}

func (s *authHttpSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	body := assertResponse(c, resp, expCode, apihttp.CTypeJSON)
	c.Check(jsonResponse(c, body).Error, gc.Matches, expError)
}

func (s *authHttpSuite) dialWebsocketFromURL(c *gc.C, server string, header http.Header) *websocket.Conn {
	config := s.makeWebsocketConfigFromURL(c, server, header)
	c.Logf("dialing %v", server)
	conn, err := websocket.DialConfig(config)
	c.Assert(err, jc.ErrorIsNil)
	return conn
}

func (s *authHttpSuite) makeWebsocketConfigFromURL(c *gc.C, server string, header http.Header) *websocket.Config {
	config, err := websocket.NewConfig(server, "http://localhost/")
	c.Assert(err, jc.ErrorIsNil)
	config.Header = header
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(testing.CACert)), jc.IsTrue)
	config.TlsConfig = &tls.Config{RootCAs: caCerts, ServerName: "anything"}
	return config
}

func (s *authHttpSuite) assertWebsocketClosed(c *gc.C, reader *bufio.Reader) {
	_, err := reader.ReadByte()
	c.Assert(err, gc.Equals, io.EOF)
}

func (s *authHttpSuite) makeURL(c *gc.C, scheme, path string, queryParams url.Values) *url.URL {
	url := s.baseURL(c)
	query := ""
	if queryParams != nil {
		query = queryParams.Encode()
	}
	url.Scheme = scheme
	url.Path += path
	url.RawQuery = query
	return url
}

// userAuthHttpSuite extends authHttpSuite with helpers for testing
// HTTP "streaming" style APIs which only accept user logins (not
// agents).
type userAuthHttpSuite struct {
	authHttpSuite
	userTag            names.UserTag
	password           string
	archiveContentType string
}

func (s *userAuthHttpSuite) SetUpTest(c *gc.C) {
	s.authHttpSuite.SetUpTest(c)
	s.password = "password"
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: s.password})
	s.userTag = user.UserTag()
}

func (s *userAuthHttpSuite) sendRequest(c *gc.C, tag, password, method, uri, contentType string, body io.Reader) (*http.Response, error) {
	c.Logf("sendRequest: %s", uri)
	req, err := http.NewRequest(method, uri, body)
	c.Assert(err, jc.ErrorIsNil)
	if tag != "" && password != "" {
		req.SetBasicAuth(tag, password)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return utils.GetNonValidatingHTTPClient().Do(req)
}

func (s *userAuthHttpSuite) setupOtherEnvironment(c *gc.C) *state.State {
	envState := s.Factory.MakeEnvironment(c, nil)
	s.AddCleanup(func(*gc.C) { envState.Close() })
	user := s.Factory.MakeUser(c, nil)
	_, err := envState.AddEnvironmentUser(user.UserTag(), s.userTag, "")
	c.Assert(err, jc.ErrorIsNil)
	s.userTag = user.UserTag()
	s.password = "password"
	s.envUUID = envState.EnvironUUID()
	return envState
}

func (s *userAuthHttpSuite) authRequest(c *gc.C, method, uri, contentType string, body io.Reader) (*http.Response, error) {
	return s.sendRequest(c, s.userTag.String(), s.password, method, uri, contentType, body)
}

func (s *userAuthHttpSuite) uploadRequest(c *gc.C, uri string, asZip bool, path string) (*http.Response, error) {
	contentType := apihttp.CTypeRaw
	if asZip {
		contentType = s.archiveContentType
	}

	if path == "" {
		return s.authRequest(c, "POST", uri, contentType, nil)
	}

	file, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()
	return s.authRequest(c, "POST", uri, contentType, file)
}

// assertJSONError checks the JSON encoded error returned by the log
// and logsink APIs matches the expected value.
func assertJSONError(c *gc.C, reader *bufio.Reader, expected string) {
	errResult := readJSONErrorLine(c, reader)
	c.Assert(errResult.Error, gc.NotNil)
	c.Assert(errResult.Error.Message, gc.Matches, expected)
}

// readJSONErrorLine returns the error line returned by the log and
// logsink APIS.
func readJSONErrorLine(c *gc.C, reader *bufio.Reader) params.ErrorResult {
	line, err := reader.ReadSlice('\n')
	c.Assert(err, jc.ErrorIsNil)
	var errResult params.ErrorResult
	err = json.Unmarshal(line, &errResult)
	c.Assert(err, jc.ErrorIsNil)
	return errResult
}
