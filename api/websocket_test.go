// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/rpc/params"
)

type websocketSuite struct{}

func TestWebsocketSuite(t *testing.T) {
	tc.Run(t, &websocketSuite{})
}

func (s *websocketSuite) TestWebsocketDialAddsServiceUnavailableError(c *tc.C) {
	tests := []struct {
		name        string
		contentType string
		body        string
		expect      string
	}{{
		name:   "no content type",
		body:   "logsink unavailable",
		expect: "logsink unavailable \\(Service Unavailable\\)",
	}, {
		name:        "plain text content type",
		contentType: "text/plain; charset=utf-8",
		body:        "logsink unavailable\n",
		expect:      "logsink unavailable \\(Service Unavailable\\)",
	}, {
		name:        "html content type",
		contentType: "text/html",
		body:        "<html>logsink unavailable</html>",
		expect:      "<html>logsink unavailable</html> \\(Service Unavailable\\)",
	}, {
		name:        "json content type",
		contentType: "application/json",
		body:        jsonErrorBody(c, "logsink unavailable"),
		expect:      "logsink unavailable",
	}, {
		name:        "json content type with charset",
		contentType: "application/json; charset=utf-8",
		body:        jsonErrorBody(c, "logsink unavailable"),
		expect:      "logsink unavailable",
	},
	}

	for _, test := range tests {
		c.Logf("test %s", test.name)
		header := make(http.Header)
		if test.contentType != "" {
			header.Set("Content-Type", test.contentType)
		}
		dialer := badHandshakeDialer{
			response: &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader(test.body)),
				Header:     header,
			},
		}

		stream, err := api.WebsocketDialWithErrors(dialer, "wss://example.invalid", nil)
		c.Assert(stream, tc.IsNil)
		c.Assert(err, tc.ErrorMatches, test.expect)
		c.Assert(err, tc.ErrorIs, api.HTTPStatusServiceUnavailable)
	}
}

func (s *websocketSuite) TestWebsocketDialDoesNotAddServiceUnavailableErrorForOtherStatus(c *tc.C) {
	dialer := badHandshakeDialer{
		response: &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("logsink failed")),
			Header:     make(http.Header),
		},
	}

	stream, err := api.WebsocketDialWithErrors(dialer, "wss://example.invalid", nil)
	c.Assert(stream, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "logsink failed \\(Internal Server Error\\)")
	c.Assert(err, tc.Not(tc.ErrorIs), api.HTTPStatusServiceUnavailable)
}

func jsonErrorBody(c *tc.C, message string) string {
	result := params.ErrorResult{
		Error: &params.Error{
			Message: message,
		},
	}
	body, err := json.Marshal(result)
	c.Assert(err, tc.ErrorIsNil)
	return string(body)
}

type badHandshakeDialer struct {
	response *http.Response
}

func (d badHandshakeDialer) Dial(_ string, _ http.Header) (*gorillaws.Conn, *http.Response, error) {
	return nil, d.response, gorillaws.ErrBadHandshake
}
