// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"strings"

	corecharm "github.com/juju/juju/core/charm"
)

// sanitizeCharmOrigin attempts to ensure that any fields we receive from
// the API are valid and juju knows about them.
func sanitizeCharmOrigin(received, requested corecharm.Origin) (corecharm.Origin, error) {
	// Platform is generally the problem at hand. We want to ensure if they
	// send "all" back for Architecture, OS or Release that we either use the
	// requested origin using that as the hint or we unset it from the requested
	// origin.
	result := received

	if result.Platform.Architecture == "all" {
		result.Platform.Architecture = ""
		if requested.Platform.Architecture != "all" {
			result.Platform.Architecture = requested.Platform.Architecture
		}
	}

	if result.Platform.Channel == "all" {
		result.Platform.Channel = ""

		if requested.Platform.Channel != "all" {
			result.Platform.Channel = requested.Platform.Channel
		}
		if result.Platform.Channel != "" {
			result.Platform.OS = requested.Platform.OS
		}
	}
	if result.Platform.OS == "all" {
		result.Platform.OS = ""
	}
	result.Platform.OS = strings.ToLower(result.Platform.OS)

	return result, nil
}
