// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"github.com/altoros/gosigma/data"
	"github.com/altoros/gosigma/https"
)

// A Error implements library error
type Error struct {
	SystemError   error       // wrapped error from underlying API
	StatusCode    int         // HTTP status code
	StatusMessage string      // HTTP status string
	ServiceError  *data.Error // Error response object from CloudSigma endpoint
}

var _ error = Error{}

// NewError creates new Error object
func NewError(r *https.Response, e error) *Error {
	if r == nil {
		if e == nil {
			return nil
		}
		return &Error{SystemError: e}
	}
	err := &Error{
		SystemError:   e,
		StatusCode:    r.StatusCode,
		StatusMessage: r.Status,
	}
	if dee, e := data.ReadError(r.Body); e == nil {
		if len(dee) > 0 {
			err.ServiceError = &dee[0]
		}
	}
	return err
}

// Error implements error interface
func (s Error) Error() string {
	if s.ServiceError != nil && s.ServiceError.Message != "" {
		var str string
		if s.StatusCode > 0 {
			str = s.StatusMessage + ", "
		}
		str += s.ServiceError.Error()
		return str
	}
	if s.StatusCode >= 400 {
		return s.StatusMessage
	}
	if s.SystemError != nil {
		return s.SystemError.Error()
	}
	if s.StatusCode > 0 {
		return s.StatusMessage
	}
	return ""
}
