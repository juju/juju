// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package https

import (
	"fmt"
	"net/http"
	"strings"
)

// Response represents HTTPS client response
type Response struct {
	*http.Response
}

// VerifyJSON checks the response has specified code and carries application/json body
func (r Response) VerifyJSON(code int) error {
	return r.Verify(code, "application/json")
}

// Verify checks the response has specified code and body with specified content type
func (r Response) Verify(code int, contentType string) error {
	if err := r.VerifyCode(code); err != nil {
		return err
	}
	if err := r.VerifyContentType(contentType); err != nil {
		return err
	}
	return nil
}

// VerifyCode checks the response has specified code
func (r Response) VerifyCode(code int) error {
	if r.StatusCode != code {
		return fmt.Errorf("expected HTTP code: %d, got code: %d, %s", code, r.StatusCode, r.Status)
	}
	return nil
}

// VerifyContentType checks the response has specified content type
func (r Response) VerifyContentType(contentType string) error {
	if contentType == "" {
		return nil
	}

	contentType = strings.ToLower(contentType)

	vv, ok := r.Header["Content-Type"]
	if !ok {
		return fmt.Errorf("header Content-Type not found in response, expected \"%s\"", contentType)
	}

	for _, v := range vv {
		v = strings.ToLower(v)
		if strings.Contains(v, contentType) {
			return nil
		}
	}

	return fmt.Errorf("expected Content-Type: \"%s\", received \"%v\"", contentType, vv)
}
