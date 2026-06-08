// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api"
)

func isLogSinkUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, api.HTTPStatusServiceUnavailable)
}
