// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package macaroon

import (
	"github.com/juju/juju/core/logger"
	jujuhttp "github.com/juju/juju/internal/http"
)

// DefaultHTTPClient returns the default http client used to access the object
// store.
func DefaultHTTPClient(logger logger.Logger) *jujuhttp.Client {
	return jujuhttp.NewClient(
		jujuhttp.WithLogger(logger),
	)
}
