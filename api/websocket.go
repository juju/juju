// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// websocketTimeout is how long we'll wait for a WriteJSON call before
// timing it out.
const websocketTimeout = 30 * time.Second

// WebsocketDial is called instead of dialer.Dial so we can override it in
// tests.
var WebsocketDial = WebsocketDialWithErrors

// WebsocketDialer is something that can make a websocket connection. Enables
// testing the error unpacking in websocketDialWithErrors.
type WebsocketDialer interface {
	Dial(string, http.Header) (*websocket.Conn, *http.Response, error)
}

// WebsocketDialWithErrors dials the websocket and extracts any error
// from the response if there's a handshake error setting up the
// socket. Any other errors are returned normally.
func WebsocketDialWithErrors(dialer WebsocketDialer, urlStr string, requestHeader http.Header) (base.Stream, error) {
	c, resp, err := dialer.Dial(urlStr, requestHeader)
	if err != nil {
		if err == websocket.ErrBadHandshake {
			// If ErrBadHandshake is returned, a non-nil response
			// is returned so the client can react to auth errors
			// (for example).
			//
			// The problem here is that there is a response, but the response
			// body is truncated to 1024 bytes for debugging information, not
			// for a true response. While this may work for small bodies, it
			// isn't guaranteed to work for all messages.
			defer resp.Body.Close()
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return nil, err
			}
			if resp.Header.Get("Content-Type") == "application/json" {
				var result params.ErrorResult
				jsonErr := json.Unmarshal(body, &result)
				if jsonErr != nil {
					return nil, errors.Annotate(jsonErr, "reading error response")
				}
				return nil, result.Error
			}

			err = errors.Errorf(
				"%s (%s)",
				strings.TrimSpace(string(body)),
				http.StatusText(resp.StatusCode),
			)
		}
		return nil, err
	}
	result := DeadlineStream{Conn: c, Timeout: websocketTimeout}
	return &result, nil
}

// DeadlineStream wraps a websocket connection and applies a write
// deadline to each WriteJSON call.
type DeadlineStream struct {
	*websocket.Conn

	Timeout time.Duration
}

// WriteJSON is part of base.Stream.
func (s *DeadlineStream) WriteJSON(v interface{}) error {
	// This uses a real clock rather than trying to use a clock passed
	// in because the websocket will use a real clock to determine
	// whether the deadline has passed anyway.
	deadline := time.Now().Add(s.Timeout)
	if err := s.Conn.SetWriteDeadline(deadline); err != nil {
		return errors.Annotate(err, "setting write deadline")
	}
	return errors.Trace(s.Conn.WriteJSON(v))
}

type UrlCatcher struct {
	location string
	headers  http.Header
}

func (u *UrlCatcher) RecordLocation(d WebsocketDialer, urlStr string, header http.Header) (base.Stream, error) {
	u.location = urlStr
	u.headers = header
	pr, pw := io.Pipe()
	go func() {
		fmt.Fprintf(pw, "null\n")
	}()
	return fakeStreamReader{pr}, nil
}

func (u *UrlCatcher) Location() string {
	return u.location
}

func (u *UrlCatcher) Headers() http.Header {
	return u.headers
}

type fakeStreamReader struct {
	io.Reader
}

func NewFakeStreamReader(r io.Reader) base.Stream {
	return fakeStreamReader{Reader: r}
}

var _ base.Stream = fakeStreamReader{}

func (s fakeStreamReader) Close() error {
	if c, ok := s.Reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (s fakeStreamReader) NextReader() (messageType int, r io.Reader, err error) {
	return websocket.TextMessage, s.Reader, nil
}

func (s fakeStreamReader) Write([]byte) (int, error) {
	return 0, errors.NotImplementedf("Write")
}

func (s fakeStreamReader) ReadJSON(v interface{}) error {
	return errors.NotImplementedf("ReadJSON")
}

func (s fakeStreamReader) WriteJSON(v interface{}) error {
	return errors.NotImplementedf("WriteJSON")
}
