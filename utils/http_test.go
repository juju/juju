// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type insecureClientSuite struct {
	Server *httptest.Server
}

var _ = gc.Suite(&insecureClientSuite{})

type trivialResponseHandler struct{}

func (t *trivialResponseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Greetings!\n")
}

func (s *insecureClientSuite) SetUpSuite(c *gc.C) {
}

func (s *insecureClientSuite) TearDownSuite(c *gc.C) {
}

func (s *insecureClientSuite) SetUpTest(c *gc.C) {
	s.Server = httptest.NewTLSServer(&trivialResponseHandler{})
}

func (s *insecureClientSuite) TearDownTest(c *gc.C) {
	if s.Server != nil {
		s.Server.Close()
	}
}

func (s *insecureClientSuite) TestDefaultClientFails(c *gc.C) {
	_, err := http.Get(s.Server.URL)
	c.Assert(err, gc.ErrorMatches, "(.|\n)*x509: certificate signed by unknown authority")
}

func (s *insecureClientSuite) TestInsecureClientSucceeds(c *gc.C) {
	response, err := utils.GetNonValidatingHTTPClient().Get(s.Server.URL)
	c.Assert(err, gc.IsNil)
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	c.Assert(err, gc.IsNil)
	c.Check(string(body), gc.Equals, "Greetings!\n")
}

func (s *insecureClientSuite) TestInsecureClientCached(c *gc.C) {
	client1 := utils.GetNonValidatingHTTPClient()
	client2 := utils.GetNonValidatingHTTPClient()
	c.Check(client1, gc.Equals, client2)
}
