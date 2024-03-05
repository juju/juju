// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"github.com/juju/errors"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

// DataError is a go error that wraps the odataerrors.ODataError response type.
type DataError struct {
	*odataerrors.ODataError
}

// Code returns the code from the wrapped DataError.
func (e *DataError) Code() string {
	return *e.ODataError.GetErrorEscaped().GetCode()
}

// Message returns the message from the wrapped DataError.
func (e *DataError) Message() string {
	msg := e.ODataError.GetErrorEscaped().GetMessage()
	if msg != nil {
		return *msg
	}
	return e.ODataError.Message
}

// Error implements the error interface.
func (e *DataError) Error() string {
	s := e.Code()
	if m := e.Message(); m != "" {
		s += ": " + m
	}
	return s
}

// AsDataError returns a wrapped error that exposes the
// underlying error code and message (if possible).
func AsDataError(err error) (*DataError, bool) {
	if err == nil {
		return nil, false
	}
	var dataErr *DataError
	if errors.As(err, &dataErr) {
		return dataErr, true
	}

	var apiErr *odataerrors.ODataError
	if !errors.As(err, &apiErr) {
		return nil, false
	}
	return &DataError{apiErr}, true
}

// ReportableError returns a wrapped error that exposes the
// underlying error code and message (if possible), or just
// the passed in error.
func ReportableError(err error) error {
	var apiErr *odataerrors.ODataError
	if !errors.As(err, &apiErr) {
		return err
	}
	return &DataError{apiErr}
}
