// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v11"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/core/base"
)

// BaseForCharm takes a requested base and a list of bases supported by a
// charm and returns the base which is relevant.
// If the requested base is empty, then the first supported base is used,
// otherwise the requested base is validated against the supported bases.
func BaseForCharm(requestedBase base.Base, supportedBases []base.Base) (base.Base, error) {
	// Old local charm with no supported bases, use the
	// requestedBase. If none specified error.
	if len(supportedBases) == 0 {
		if requestedBase.Empty() {
			return base.Base{}, errMissingBase
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
	return base.Base{}, NewUnsupportedBaseError(requestedBase, supportedBases)
}

// errMissingBase is used to denote that BaseForCharm could not determine
// a base because a legacy charm did not declare any.
var errMissingBase = errors.New("base not specified and charm does not define any")

// IsMissingBaseError returns true if err is an errMissingBase.
func IsMissingBaseError(err error) bool {
	return err == errMissingBase
}

// ComputedBases of a charm, preserving legacy behaviour. For charms prior to v2,
// fall back the metadata series can convert to bases
func ComputedBases(c charm.CharmMeta) ([]base.Base, error) {
	manifest := c.Manifest()
	if manifest != nil {
		computedBases := make([]base.Base, len(manifest.Bases))
		for i, b := range manifest.Bases {
			computedBase, err := base.ParseBase(b.Name, b.Channel.String())
			if err != nil {
				return nil, errors.Trace(err)
			}
			computedBases[i] = computedBase
		}
		return computedBases, nil
	}
	if charm.MetaFormat(c) < charm.FormatV2 {
		return transform.SliceOrErr(c.Meta().Series, base.GetBaseFromSeries)
	}
	return []base.Base{}, nil
}
