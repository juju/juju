// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

type proxyConnectError struct {
	error
	proxyType string
}

// NewProxyConnectError returns a proxy connect error.
func NewProxyConnectError(err error, proxyType string) *proxyConnectError {
	return &proxyConnectError{error: err, proxyType: proxyType}
}

// IsProxyConnectError reports whether err
// was created with NewProxyConnectError.
func IsProxyConnectError(err error) bool {
	err = errors.Cause(err)
	_, ok := err.(*proxyConnectError)
	return ok
}

// ProxyType returns the type of proxy which generated
// the specified proxy connect error.
func ProxyType(err error) string {
	proxyErr, ok := errors.Cause(err).(*proxyConnectError)
	if !ok {
		return ""
	}
	return proxyErr.proxyType
}
