// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import (
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/juju/juju/version"
)

const (
	// contentTypeTextUTF8 represents the content type value for plain text in
	// utf-8 encoding.
	contentTypeTextUTF8 = "text/plain;charset=utf-8"

	// headerAccept is the accept key used in http headers.
	headerAccept = "Accept"

	// headerContentType is the content type key used in http headers.
	headerContentType = "Content-Type"

	// headerUserAgent is the user agent key used in http headers.
	headerUserAgent = "User-Agent"
)

// Client represents a http client capable of performing a http request.
type Client interface {
	// Do is responsible for performing a [http.Request] and returning the
	// subsequent result. See [http.Client.Do] for documentation on this func.
	Do(*http.Request) (*http.Response, error)
}

// clientFunc is a convience method for implementing the [Client] interface as
// a func.
type clientFunc func(*http.Request) (*http.Response, error)

// Do implements the [Client] interface for [clientFunc].
func (c clientFunc) Do(r *http.Request) (*http.Response, error) {
	return c(r)
}

// hasContentType checks to see a [http.Response] Content-Type contains the
// supplied content type.
func hasContentType(res *http.Response, contentType string) (bool, error) {
	contentType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false, fmt.Errorf(
			"parsing has Content-Type %q: %w",
			contentType,
			err,
		)
	}

	val := res.Header.Get(headerContentType)
	if val == "" {
		return false, nil
	}

	resContentType, _, err := mime.ParseMediaType(val)
	if err != nil {
		return false, fmt.Errorf(
			"parsing response Content-Type %q: %w",
			val,
			err,
		)
	}

	return contentType == resContentType, nil
}

// setAccept adds to a [http.Request] headers zero or more accepted content
// types that a request will accept.
func setAccept(req *http.Request, types ...string) *http.Request {
	if len(types) == 0 {
		return req
	}
	req.Header.Add(headerAccept, strings.Join(types, ", "))
	return req
}

// userAgentSetter intercepts a http client request and sets the user agent
// header.
func userAgentSetter(client Client) clientFunc {
	return func(req *http.Request) (*http.Response, error) {
		req.Header.Add(
			headerUserAgent,
			fmt.Sprintf("juju/%s ssh key importer", version.Current.String()),
		)
		return client.Do(req)
	}
}
