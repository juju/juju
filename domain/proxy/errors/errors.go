// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ProxyInfoNotSupported describes an error that occurs when the underlying
	// provider does not support getting the proxy information.
	ProxyInfoNotSupported = errors.ConstError("proxy info not supported")
	// ProxyInfoNotFound describes an error that occurs when a proxy information
	// cannot be found.
	ProxyInfoNotFound = errors.ConstError("proxy info not found")
	// ProxyNotSupported describes an error that occurs when the underlying
	// provider does not support the proxy.
	ProxyNotSupported = errors.ConstError("proxy not supported")
)
