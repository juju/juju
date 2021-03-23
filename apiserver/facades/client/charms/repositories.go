// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/v2/series"

	corecharm "github.com/juju/juju/core/charm"
)

var logger = loggo.GetLogger("juju.apiserver.charms")

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

	if result.Platform.OS == "all" {
		result.Platform.OS = ""
	}

	if result.Platform.Series == "all" {
		result.Platform.Series = ""

		if requested.Platform.Series != "all" {
			result.Platform.Series = requested.Platform.Series
		}
	}

	if result.Platform.Series != "" {
		os, err := series.GetOSFromSeries(result.Platform.Series)
		if err != nil {
			return result, errors.Trace(err)
		}
		result.Platform.OS = strings.ToLower(os.String())
	}

	return result, nil
}

// Metadata represents the return type for both charm types (charm and bundles)
type Metadata interface {
	ComputedSeries() []string
}

type bundleMetadata struct {
	*charm.BundleData
}

func (b bundleMetadata) ComputedSeries() []string {
	return []string{b.BundleData.Series}
}
