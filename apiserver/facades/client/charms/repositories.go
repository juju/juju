// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"strings"

	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/v2/series"

	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/apiserver/params"
	corecharm "github.com/juju/juju/core/charm"
)

var logger = loggo.GetLogger("juju.apiserver.charms")

// sanitizeCharmOrigin attempts to ensure that any fields we receive from
// the API are valid and juju knows about them.
func sanitizeCharmOrigin(received, requested params.CharmOrigin) (params.CharmOrigin, error) {
	// Platform is generally the problem at hand. We want to ensure if they
	// send "all" back for Architecture, OS or Series that we either use the
	// requested origin using that as the hint or we unset it from the requested
	// origin.
	result := received

	if result.Architecture == "all" {
		result.Architecture = ""
		if requested.Architecture != "all" {
			result.Architecture = requested.Architecture
		}
	}

	if result.OS == "all" {
		result.OS = ""
	}

	if result.Series == "all" {
		result.Series = ""

		if requested.Series != "all" {
			result.Series = requested.Series
		}
	}

	if result.Series != "" {
		os, err := series.GetOSFromSeries(result.Series)
		if err != nil {
			return result, errors.Trace(err)
		}
		result.OS = strings.ToLower(os.String())
	}

	return result, nil
}

func sanitizeCoreCharmOrigin(received, requested corecharm.Origin) (corecharm.Origin, error) {
	a := apicharm.CoreCharmOrigin(received)
	b := apicharm.CoreCharmOrigin(requested)
	res, err := sanitizeCharmOrigin(a.ParamsCharmOrigin(), b.ParamsCharmOrigin())
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}
	return apicharm.APICharmOrigin(res).CoreCharmOrigin(), nil
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
