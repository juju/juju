// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"net/http"
)

// HTTPError represents an error caused by a failed http request.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e HTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	} else {
		return fmt.Sprintf("%d: %s", e.StatusCode, "request failed")
	}
}

// NotAvailError indicates that the service is either unreachable or unavailable.
type NotAvailError struct {
	StatusCode int
}

func (e NotAvailError) Error() string {
	if e.StatusCode == http.StatusServiceUnavailable {
		return "service unavailable"
	} else {
		return "service unreachable"
	}
}

// IsNotAvail indicates whether the error is a NotAvailError.
func IsNotAvail(err error) bool {
	_, ok := err.(NotAvailError)
	return ok
}
