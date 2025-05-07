// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/juju/tc"
	"github.com/juju/testing"

	jujuhttp "github.com/juju/juju/internal/http"
)

type httpSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&httpSuite{})

func (s *httpSuite) TestBasicAuthHeader(c *tc.C) {
	header := jujuhttp.BasicAuthHeader("eric", "sekrit")
	c.Assert(len(header), tc.Equals, 1)
	auth := header.Get("Authorization")
	fields := strings.Fields(auth)
	c.Assert(len(fields), tc.Equals, 2)
	basic, encoded := fields[0], fields[1]
	c.Assert(basic, tc.Equals, "Basic")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	c.Assert(err, tc.IsNil)
	c.Assert(string(decoded), tc.Equals, "eric:sekrit")
}

func (s *httpSuite) TestParseBasicAuthHeader(c *tc.C) {
	tests := []struct {
		about          string
		h              http.Header
		expectUserid   string
		expectPassword string
		expectError    string
	}{{
		about:       "no Authorization header",
		h:           http.Header{},
		expectError: "invalid or missing HTTP auth header",
	}, {
		about: "empty Authorization header",
		h: http.Header{
			"Authorization": {""},
		},
		expectError: "invalid or missing HTTP auth header",
	}, {
		about: "Not basic encoding",
		h: http.Header{
			"Authorization": {"NotBasic stuff"},
		},
		expectError: "invalid or missing HTTP auth header",
	}, {
		about: "invalid base64",
		h: http.Header{
			"Authorization": {"Basic not-base64"},
		},
		expectError: "invalid HTTP auth encoding",
	}, {
		about: "no ':'",
		h: http.Header{
			"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("aladdin"))},
		},
		expectError: "invalid HTTP auth contents",
	}, {
		about: "valid credentials",
		h: http.Header{
			"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("aladdin:open sesame"))},
		},
		expectUserid:   "aladdin",
		expectPassword: "open sesame",
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		u, p, err := jujuhttp.ParseBasicAuthHeader(test.h)
		c.Assert(u, tc.Equals, test.expectUserid)
		c.Assert(p, tc.Equals, test.expectPassword)
		if test.expectError != "" {
			c.Assert(err.Error(), tc.Equals, test.expectError)
		} else {
			c.Assert(err, tc.IsNil)
		}
	}
}
