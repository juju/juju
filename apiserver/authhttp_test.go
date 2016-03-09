// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// authHttpSuite provides helpers for testing HTTP "streaming" style APIs.
type authHttpSuite struct {
	// macaroonAuthEnabled may be set by a test suite
	// before SetUpTest is called. If it is true, macaroon
	// authentication will be enabled for the duration
	// of the suite.
	macaroonAuthEnabled bool

	// MacaroonSuite is embedded because we need
	// it when macaroonAuthEnabled is true.
	// When macaroonAuthEnabled is false,
	// only the JujuConnSuite in it will be initialized;
	// all other fields will be zero.
	apitesting.MacaroonSuite

	modelUUID string

	// userTag and password hold the user tag and password
	// to use in authRequest. When macaroonAuthEnabled
	// is true, password will be empty.
	userTag  names.UserTag
	password string
}

func (s *authHttpSuite) SetUpTest(c *gc.C) {
	if s.macaroonAuthEnabled {
		s.MacaroonSuite.SetUpTest(c)
	} else {
		// No macaroons, so don't enable them.
		s.JujuConnSuite.SetUpTest(c)
	}

	s.modelUUID = s.State.ModelUUID()

	if s.macaroonAuthEnabled {
		// When macaroon authentication is enabled, we must use
		// an external user.
		s.userTag = names.NewUserTag("bob@authhttpsuite")
		s.AddModelUser(c, s.userTag.Id())
	} else {
		// Make a user in the state.
		s.password = "password"
		user := s.Factory.MakeUser(c, &factory.UserParams{Password: s.password})
		s.userTag = user.UserTag()
	}
}

func (s *authHttpSuite) TearDownTest(c *gc.C) {
	if s.macaroonAuthEnabled {
		s.MacaroonSuite.TearDownTest(c)
	} else {
		s.JujuConnSuite.TearDownTest(c)
	}
}

func (s *authHttpSuite) baseURL(c *gc.C) *url.URL {
	info := s.APIInfo(c)
	return &url.URL{
		Scheme: "https",
		Host:   info.Addrs[0],
		Path:   "",
	}
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

// httpRequestParams holds parameters for the authRequest and sendRequest
// methods.
type httpRequestParams struct {
	// do is used to make the HTTP request.
	// If it is nil, utils.GetNonValidatingHTTPClient().Do will be used.
	// If the body reader implements io.Seeker,
	// req.Body will also implement that interface.
	do func(req *http.Request) (*http.Response, error)

	// expectError holds the error regexp to match
	// against the error returned from the HTTP Do
	// request. If it is empty, the error is expected to be
	// nil.
	expectError string

	// tag holds the tag to authenticate as.
	tag string

	// password holds the password associated with the tag.
	password string

	// method holds the HTTP method to use for the request.
	method string

	// url holds the URL to send the HTTP request to.
	url string

	// contentType holds the content type of the request.
	contentType string

	// body holds the body of the request.
	body io.Reader

	// jsonBody holds an object to be marshaled as JSON
	// as the body of the request. If this is specified, body will
	// be ignored and the Content-Type header will
	// be set to application/json.
	jsonBody interface{}

	// nonce holds the machine nonce to provide in the header.
	nonce string
}

func (s *authHttpSuite) sendRequest(c *gc.C, p httpRequestParams) *http.Response {
	c.Logf("sendRequest: %s", p.url)
	hp := httptesting.DoRequestParams{
		Do:          p.do,
		Method:      p.method,
		URL:         p.url,
		Body:        p.body,
		JSONBody:    p.jsonBody,
		Header:      make(http.Header),
		Username:    p.tag,
		Password:    p.password,
		ExpectError: p.expectError,
	}
	if p.contentType != "" {
		hp.Header.Set("Content-Type", p.contentType)
	}
	if p.nonce != "" {
		hp.Header.Set(params.MachineNonceHeader, p.nonce)
	}
	if hp.Do == nil {
		hp.Do = utils.GetNonValidatingHTTPClient().Do
	}
	return httptesting.Do(c, hp)
}

// bakeryDo provides a function suitable for using in httpRequestParams.Do
// that will use the given http client (or utils.GetNonValidatingHTTPClient()
// if client is nil) and use the given getBakeryError function
// to translate errors in responses.
func bakeryDo(client *http.Client, getBakeryError func(*http.Response) error) func(*http.Request) (*http.Response, error) {
	bclient := httpbakery.NewClient()
	if client != nil {
		bclient.Client = client
	} else {
		// Configure the default client to skip verification/
		bclient.Client.Transport = utils.NewHttpTLSTransport(&tls.Config{
			InsecureSkipVerify: true,
		})
	}
	return func(req *http.Request) (*http.Response, error) {
		var body io.ReadSeeker
		if req.Body != nil {
			body = req.Body.(io.ReadSeeker)
			req.Body = nil
		}
		return bclient.DoWithBodyAndCustomError(req, body, getBakeryError)
	}
}

// authRequest is like sendRequest but fills out p.tag and p.password
// from the userTag and password fields in the suite.
func (s *authHttpSuite) authRequest(c *gc.C, p httpRequestParams) *http.Response {
	p.tag = s.userTag.String()
	p.password = s.password
	return s.sendRequest(c, p)
}

func (s *authHttpSuite) setupOtherModel(c *gc.C) *state.State {
	envState := s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { envState.Close() })
	user := s.Factory.MakeUser(c, nil)
	_, err := envState.AddModelUser(state.ModelUserSpec{
		User:      user.UserTag(),
		CreatedBy: s.userTag})
	c.Assert(err, jc.ErrorIsNil)
	s.userTag = user.UserTag()
	s.password = "password"
	s.modelUUID = envState.ModelUUID()
	return envState
}

func (s *authHttpSuite) uploadRequest(c *gc.C, uri string, contentType, path string) *http.Response {
	if path == "" {
		return s.authRequest(c, httpRequestParams{
			method:      "POST",
			url:         uri,
			contentType: contentType,
		})
	}

	file, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()
	return s.authRequest(c, httpRequestParams{
		method:      "POST",
		url:         uri,
		contentType: contentType,
		body:        file,
	})
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

func assertResponse(c *gc.C, resp *http.Response, expHTTPStatus int, expContentType string) []byte {
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resp.StatusCode, gc.Equals, expHTTPStatus, gc.Commentf("body: %s", body))
	ctype := resp.Header.Get("Content-Type")
	c.Assert(ctype, gc.Equals, expContentType)
	return body
}

// bakeryGetError implements a getError function
// appropriate for passing to httpbakery.Client.DoWithBodyAndCustomError
// for any endpoint that returns the error in a top level Error field.
func bakeryGetError(resp *http.Response) error {
	if resp.StatusCode != http.StatusUnauthorized {
		return nil
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Annotatef(err, "cannot read body")
	}
	var errResp params.ErrorResult
	if err := json.Unmarshal(data, &errResp); err != nil {
		return errors.Annotatef(err, "cannot unmarshal body")
	}
	if errResp.Error == nil {
		return errors.New("no error found in error response body")
	}
	if errResp.Error.Code != params.CodeDischargeRequired {
		return errResp.Error
	}
	if errResp.Error.Info == nil {
		return errors.Annotatef(err, "no error info found in discharge-required response error")
	}
	// It's a discharge-required error, so make an appropriate httpbakery
	// error from it.
	return &httpbakery.Error{
		Message: errResp.Error.Message,
		Code:    httpbakery.ErrDischargeRequired,
		Info: &httpbakery.ErrorInfo{
			Macaroon:     errResp.Error.Info.Macaroon,
			MacaroonPath: errResp.Error.Info.MacaroonPath,
		},
	}
}
