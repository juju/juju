// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

// BasicAuthHeader creates a header that contains just the "Authorization"
// entry.  The implementation was originally taked from net/http but this is
// needed externally from the http request object in order to use this with
// our websockets. See 2 (end of page 4) http://www.ietf.org/rfc/rfc2617.txt
// "To receive authorization, the client sends the userid and password,
// separated by a single colon (":") character, within a base64 encoded string
// in the credentials."
func BasicAuthHeader(username, password string) http.Header {
	auth := username + ":" + password
	encoded := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	return http.Header{
		"Authorization": {encoded},
	}
}

// ParseBasicAuth attempts to find an Authorization header in the supplied
// http.Header and if found parses it as a Basic header. See 2 (end of page 4)
// http://www.ietf.org/rfc/rfc2617.txt "To receive authorization, the client
// sends the userid and password, separated by a single colon (":") character,
// within a base64 encoded string in the credentials."
func ParseBasicAuthHeader(h http.Header) (userid, password string, err error) {
	parts := strings.Fields(h.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Basic" {
		return "", "", fmt.Errorf("invalid or missing HTTP auth header")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("invalid HTTP auth encoding")
	}
	tokens := strings.SplitN(string(challenge), ":", 2)
	if len(tokens) != 2 {
		return "", "", fmt.Errorf("invalid HTTP auth contents")
	}
	return tokens[0], tokens[1], nil
}
