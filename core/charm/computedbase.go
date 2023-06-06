// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v11"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/core/series"
)

// BaseForCharm takes a requested series and a list of series supported by a
// charm and returns the series which is relevant.
// If the requested base is empty, then the first supported base is used,
// otherwise the requested base is validated against the supported base.
func BaseForCharm(requestedBase series.Base, supportedBases []series.Base) (series.Base, error) {
	// Old local charm with no supported bases, use the
	// requestedBase. If none specified error.
	if len(supportedBases) == 0 {
		if requestedBase.Empty() {
			return series.Base{}, errMissingBase
		}
		return requestedBase, nil
	}
	// Use the charm default.
	if requestedBase.Empty() {
		return supportedBases[0], nil
	}
	for _, s := range supportedBases {
		if s.IsCompatible(requestedBase) {
			return requestedBase, nil
		}
	}
	return series.Base{}, NewUnsupportedBaseError(requestedBase, supportedBases)
}

// errMissingBase is used to denote that BaseForCharm could not determine
// a base because a legacy charm did not declare any.
var errMissingBase = errors.New("base not specified and charm does not define any")

// IsMissingBaseError returns true if err is an errMissingBase.
func IsMissingBaseError(err error) bool {
	return err == errMissingBase
}

// ComputedBases of a charm, preserving legacy behaviour. If a charm has
// no manifest, fall back to ComputedSeries and attempt to convert to a
// base
func ComputedBases(c charm.CharmMeta) ([]series.Base, error) {
	manifest := c.Manifest()
	if manifest != nil {
		computedBases := make([]series.Base, len(manifest.Bases))
		for i, base := range manifest.Bases {
			computedBase, err := series.ParseBase(base.Name, base.Channel.String())
			if err != nil {
				return nil, errors.Trace(err)
			}
			computedBases[i] = computedBase
		}
		return computedBases, nil
	}

	computedSeries, err := ComputedSeries(c)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return transform.SliceOrErr(computedSeries, series.GetBaseFromSeries)
}
