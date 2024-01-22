// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	jujuhttp "github.com/juju/http/v2"
)

// DefaultHTTPClient returns the default http client used to access the object
// store.
func DefaultHTTPClient(logger Logger) HTTPClient {
	return jujuhttp.NewClient(
		jujuhttp.WithLogger(logger),
	)
}
