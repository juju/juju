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

	// Another problem area is the origin channel. We desire a full channel (track and risk).
	// However, as a result of a charmhub bug, sometimes the received charm origin has no track in it's
	// channel. This happens when we resolve a charm whose default channel track is 'latest', and we do
	// not specify a specific channel in our refresh request.  This only happens for 'latest' track, so
	// if we have no track, we know we can fill it in as 'latest' to counteract this bug
	if result.Channel != nil && result.Channel.Track == "" {
		result.Channel.Track = "latest"
	}

	return result, nil
}
