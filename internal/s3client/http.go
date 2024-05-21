// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"github.com/juju/juju/core/logger"
	jujuhttp "github.com/juju/juju/internal/http"
)

// DefaultHTTPClient returns the default http client used to access the object
// store.
func DefaultHTTPClient(logger logger.Logger) HTTPClient {
	return jujuhttp.NewClient(
		jujuhttp.WithLogger(logger),
	)
}
