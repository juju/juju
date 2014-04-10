// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type httpSuite struct {
	Server *httptest.Server
}

var _ = gc.Suite(&httpSuite{})

type trivialResponseHandler struct{}

func (t *trivialResponseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Greetings!\n")
}

func (s *httpSuite) SetUpSuite(c *gc.C) {
}

func (s *httpSuite) TearDownSuite(c *gc.C) {
}

func (s *httpSuite) SetUpTest(c *gc.C) {
	s.Server = httptest.NewTLSServer(&trivialResponseHandler{})
}

func (s *httpSuite) TearDownTest(c *gc.C) {
	if s.Server != nil {
		s.Server.Close()
	}
}

func (s *httpSuite) TestDefaultClientFails(c *gc.C) {
	_, err := http.Get(s.Server.URL)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
}

func (s *httpSuite) TestValidatingClientGetter(c *gc.C) {
	client1 := utils.GetValidatingHTTPClient()
	client2 := utils.GetHTTPClient(utils.VerifySSLHostnames)
	c.Check(client1, gc.Equals, client2)
}

func (s *httpSuite) TestNonValidatingClientGetter(c *gc.C) {
	client1 := utils.GetNonValidatingHTTPClient()
	client2 := utils.GetHTTPClient(utils.NoVerifySSLHostnames)
	c.Check(client1, gc.Equals, client2)
}

func (s *httpSuite) TestValidatingClientFails(c *gc.C) {
	client := utils.GetValidatingHTTPClient()
	_, err := client.Get(s.Server.URL)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
}

func (s *httpSuite) TestInsecureClientSucceeds(c *gc.C) {
	response, err := utils.GetNonValidatingHTTPClient().Get(s.Server.URL)
	c.Assert(err, gc.IsNil)
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	c.Assert(err, gc.IsNil)
	c.Check(string(body), gc.Equals, "Greetings!\n")
}

func (s *httpSuite) TestInsecureClientCached(c *gc.C) {
	client1 := utils.GetNonValidatingHTTPClient()
	client2 := utils.GetNonValidatingHTTPClient()
	c.Check(client1, gc.Equals, client2)
}

func (s *httpSuite) TestBasicAuthHeader(c *gc.C) {
	header := utils.BasicAuthHeader("eric", "sekrit")
	c.Assert(len(header), gc.Equals, 1)
	auth := header.Get("Authorization")
	fields := strings.Fields(auth)
	c.Assert(len(fields), gc.Equals, 2)
	basic, encoded := fields[0], fields[1]
	c.Assert(basic, gc.Equals, "Basic")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	c.Assert(err, gc.IsNil)
	c.Assert(string(decoded), gc.Equals, "eric:sekrit")
}
