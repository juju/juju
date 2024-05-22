// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/charm"
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
			return base.Base{}, MissingBaseError
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

// MissingBaseError is used to denote that BaseForCharm could not determine
// a base because a legacy charm did not declare any.
var MissingBaseError = errors.ConstError("charm does not define any bases")

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

// BaseIsCompatibleWithCharm returns nil if the provided charm is compatible
// with the provided base. Otherwise, return an UnsupportedBaseError
func BaseIsCompatibleWithCharm(b base.Base, c charm.CharmMeta) error {
	supportedBases, err := ComputedBases(c)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = BaseForCharm(b, supportedBases)
	return err
}

// OSIsCompatibleWithCharm returns nil is any of the bases the charm supports
// has an os which matched the provided os. Otherwise, return a NotSupported error
func OSIsCompatibleWithCharm(os string, c charm.CharmMeta) error {
	supportedBases, err := ComputedBases(c)
	if err != nil {
		return errors.Trace(err)
	}
	if len(supportedBases) == 0 {
		return MissingBaseError
	}
	oses := set.NewStrings(transform.Slice(supportedBases, func(b base.Base) string { return b.OS })...)
	if oses.Contains(os) {
		return nil
	}
	osesStr := strings.Join(oses.SortedValues(), "")
	errStr := fmt.Sprintf("OS %q not supported by charm %q, supported OSes are: %s", os, c.Meta().Name, osesStr)
	return errors.NewNotSupported(nil, errStr)
}
