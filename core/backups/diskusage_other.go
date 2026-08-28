// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !linux

package backups

import "math"

// diskFree returns a value indicating unlimited free space on
// platforms where it cannot be determined.
func diskFree(string) (uint64, error) {
	return math.MaxUint64, nil
}

// diskTotal returns a value indicating an unlimited disk size on
// platforms where it cannot be determined.
func diskTotal(string) (uint64, error) {
	return math.MaxUint64, nil
}
