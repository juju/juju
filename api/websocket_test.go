// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
)

type websocketSuite struct{}

func TestWebsocketSuite(t *testing.T) {
	tc.Run(t, &websocketSuite{})
}

func (s *websocketSuite) TestWebsocketDialAddsServiceUnavailableError(c *tc.C) {
	dialer := badHandshakeDialer{
		response: &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("logsink unavailable")),
			Header:     make(http.Header),
		},
	}

	stream, err := api.WebsocketDialWithErrors(dialer, "wss://example.invalid", nil)
	c.Assert(stream, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "logsink unavailable \\(Service Unavailable\\)")
	c.Assert(err, tc.ErrorIs, api.HTTPStatusServiceUnavailable)
}

type badHandshakeDialer struct {
	response *http.Response
}

func (d badHandshakeDialer) Dial(_ string, _ http.Header) (*gorillaws.Conn, *http.Response, error) {
	return nil, d.response, gorillaws.ErrBadHandshake
}
