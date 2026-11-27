// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"sort"

	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/errors"
)

// ValidateK8sUnitRange checks that the Alive units form a contiguous range for
// the application and that ordinal history does not prevent appending to it.
// highWater is the highest ordinal in the sequence, units or pods. An empty
// range may retain any high-water mark for subsequent scaling from zero.
// Invalid ranges return an error satisfying
// [applicationerrors.ScalingStateInconsistent].
func ValidateK8sUnitRange(appName string, alive []coreunit.Name, highWater uint64) error {
	ordinals := make([]int, len(alive))
	for i, name := range alive {
		if err := name.Validate(); err != nil || name.Application() != appName {
			return errors.Errorf("invalid unit name %q: %w", name, applicationerrors.ScalingStateInconsistent)
		}
		ordinals[i] = name.Number()
	}
	sort.Ints(ordinals)
	for i := 1; i < len(ordinals); i++ {
		if ordinals[i]-ordinals[i-1] != 1 {
			return errors.Errorf("unit range is not contiguous between ordinals %d and %d: %w",
				ordinals[i-1], ordinals[i], applicationerrors.ScalingStateInconsistent)
		}
	}
	if len(ordinals) > 0 && highWater > uint64(ordinals[len(ordinals)-1]) {
		return errors.Errorf("unit sequence %d exceeds highest Alive ordinal %d: %w",
			highWater, ordinals[len(ordinals)-1], applicationerrors.ScalingStateInconsistent)
	}
	return nil
}
