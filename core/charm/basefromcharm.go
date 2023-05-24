// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
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

// errMissingSeries is used to denote that BaseForCharm could not determine
// a series because a legacy charm did not declare any.
var errMissingBase = errors.New("base not specified and charm does not define any")

// IsMissingSeriesError returns true if err is an errMissingBase.
func IsMissingBaseError(err error) bool {
	return err == errMissingBase
}
